/*
 * Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	"github.com/NearNodeFlash/nnf-dm/controllers/metrics"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

// RsyncNodeDataMovementReconciler reconciles a RsyncNodeDataMovement object
type RsyncNodeDataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	contexts sync.Map
}

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress
type RsyncNodeDataMovementContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts,verbs=get;list
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts/status,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the RsyncNodeDataMovement object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RsyncNodeDataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	metrics.NnfDmRsyncNodeDataMovementReconcilesTotal.Inc()

	rsyncNode := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, rsyncNode); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Prevent gratuitous wakeups for an rsync resource that is already finished.
	if rsyncNode.Status.State == nnfv1alpha1.DataMovementConditionTypeFinished {
		return ctrl.Result{}, nil
	}

	// Perform the data movement cancellation, if requested
	if rsyncNode.Spec.Cancel {

		// Check for the scenario where a request is canceled before the DM has started.
		// If so, record it as cancelled and do nothing
		if rsyncNode.Status.StartTime.IsZero() {
			rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
			rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonCancelled
			rsyncNode.Status.EndTime = metav1.NowMicro()

			if err := r.Status().Update(ctx, rsyncNode); err != nil {
				log.Error(err, "failed to update cancelled rsync node before data movement was initiated")
				return ctrl.Result{}, err
			}

			log.Info("Cancel initiated before data movement started, doing nothing")
			return ctrl.Result{}, nil
		}

		// Retrieve the context and cancel function from the map
		dmCancelCtxI, found := r.contexts.Load(rsyncNode.Name)
		if !found {
			log.Info("Cancel requested but could not lookup cancel function.")
			return ctrl.Result{}, nil
		}
		dmCancelCtx := dmCancelCtxI.(RsyncNodeDataMovementContext)

		if errors.Is(dmCancelCtx.ctx.Err(), context.Canceled) {
			log.Info("Cancel was already initiated, doing nothing")
			return ctrl.Result{}, nil
		}

		// Perform the cancel to kill the goroutine; wait for it
		dmCancelCtx.cancel()
		<-dmCancelCtx.ctx.Done()
		log.Info("Rsync cancel request received and initiated")

		return ctrl.Result{}, nil
	}

	// Make sure if the DM is already running that we don't start up another migration
	if rsyncNode.Status.State == nnfv1alpha1.DataMovementConditionTypeRunning {
		return ctrl.Result{}, nil
	}

	// Record that this rsync node data movement operation was received and is running. It is important
	// to record that the request started so we can reliably return status to any entity that is monitoring
	// this job (like a compute node), differentiating from a pending request and a received request.
	rsyncNode.Status.StartTime = metav1.NowMicro()
	rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning
	rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess
	if err := r.Status().Update(ctx, rsyncNode); err != nil {
		log.Error(err, "failed to set rsync node as running")
		return ctrl.Result{}, err
	}

	// Expand the context with cancel and store it in the map so the cancel
	// function can be used in another reconciler loop. Also add NamespacedName
	// so we can retrieve the resource.
	ctxCancel, cancel := context.WithCancel(ctx)
	r.contexts.Store(rsyncNode.Name, RsyncNodeDataMovementContext{
		ctx:    ctxCancel,
		cancel: cancel,
	})

	// Create the function to use for the goroutine. Pass in the context and
	// rsyncnode to set the appropriate arguments.
	performDM := func(ctx context.Context, rsyncNode *dmv1alpha1.RsyncNodeDataMovement) {
		// Build the rsync command
		source := rsyncNode.Spec.Source
		destination := rsyncNode.Spec.Destination

		arguments := []string{}
		arguments = append(arguments, "--archive")
		arguments = append(arguments, source)
		arguments = append(arguments, destination)
		if rsyncNode.Spec.DryRun {
			arguments = append(arguments, "--dry-run")
		}

		var cmd *exec.Cmd
		if rsyncNode.Spec.Simulate != "" {
			log.Info("Simulating Data Movement", "command", rsyncNode.Spec.Simulate)
			cmd = exec.CommandContext(ctxCancel, "sh", "-c", rsyncNode.Spec.Simulate)
		} else {
			cmd = exec.CommandContext(ctxCancel, "rsync", arguments...)
		}

		// Set the appropriate permissions
		if rsyncNode.Spec.UserId != uint32(os.Getuid()) || rsyncNode.Spec.GroupId != uint32(os.Getgid()) {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid: rsyncNode.Spec.UserId,
					Gid: rsyncNode.Spec.GroupId,
				},
			}
		}
		log.Info("Rsync credential", "uid", rsyncNode.Spec.UserId, "gid", rsyncNode.Spec.GroupId)

		// Run the command and wait for completion
		log.Info("Executing rsync command", "source", source, "destination", destination)
		out, cmdErr := cmd.CombinedOutput()

		// Log output based on result
		if errors.Is(ctxCancel.Err(), context.Canceled) {
			log.Info("Rsync cancelled", "error", cmdErr, "output", string(out))
		} else if cmdErr != nil {
			log.Info("Rsync failure", "error", cmdErr, "output", string(out))
		} else {
			log.Info("rsync completed", "output", string(out))
		}

		// Record the completion status and retry on conflicts
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			rsyncNode = &dmv1alpha1.RsyncNodeDataMovement{}
			if err := r.Get(ctx, req.NamespacedName, rsyncNode); err != nil {
				return err
			}

			rsyncNode.Status.EndTime = metav1.NowMicro()
			rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished

			if errors.Is(ctxCancel.Err(), context.Canceled) {
				rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonCancelled
			} else if cmdErr != nil {
				rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonFailed
				rsyncNode.Status.Message = cmdErr.Error()
			} else {
				rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess
			}

			return r.Status().Update(ctx, rsyncNode)
		})
		if err != nil {
			log.Error(err, "failed to update rsync status with completion")
			// TODO Add prometheus counter to track occurrences
		} else {
			// Cancel context is no longer needed
			r.deleteCancelCtx(rsyncNode.Name)
		}
	}
	go performDM(ctx, rsyncNode)

	return ctrl.Result{}, nil
}

func (r *RsyncNodeDataMovementReconciler) deleteCancelCtx(name string) {
	if ctxToDeleteI, ok := r.contexts.LoadAndDelete(name); ok {
		ctxToDelete := ctxToDeleteI.(RsyncNodeDataMovementContext)
		ctxToDelete.cancel() // ensure resources are freed
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RsyncNodeDataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncNodeDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 128}).
		Complete(r)
}
