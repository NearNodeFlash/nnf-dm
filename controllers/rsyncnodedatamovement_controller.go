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
	"os"
	"os/exec"
	"sync"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

// RsyncNodeDataMovementReconciler reconciles a RsyncNodeDataMovement object
type RsyncNodeDataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	completions sync.Map
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts,verbs=get;list
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts/status,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RsyncNodeDataMovement object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RsyncNodeDataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	rsyncNode := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, rsyncNode); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Prevent gratuitous wakeups for an rsync resource that is already finished.
	if rsyncNode.Status.State == nnfv1alpha1.DataMovementConditionTypeFinished {
		return ctrl.Result{}, nil
	}

	if !rsyncNode.Status.StartTime.IsZero() && rsyncNode.Status.EndTime.IsZero() {

		// The rsync operation may have completed but we failed to record the completion status
		// due to an error (most likely a resource conflict). Check if the reconciler has a
		// record of the completed rsync operation, and return the results if found.
		completedRsyncNode, found := r.loadCompletion(rsyncNode.Name)

		if found {
			if err := r.Status().Update(ctx, &completedRsyncNode); err != nil {
				log.Error(err, "failed to update completed rsync node")
				return ctrl.Result{}, err
			}

			r.deleteCompletion(rsyncNode.Name)

			return ctrl.Result{}, nil
		}
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

	arguments := []string{}
	if rsyncNode.Spec.DryRun {
		arguments = append(arguments, "--dry-run")
	}

	source := rsyncNode.Spec.Source
	destination := rsyncNode.Spec.Destination
	log.Info("Executing rsync command", "source", source, "destination", destination)

	arguments = append(arguments, "--archive")
	arguments = append(arguments, source)
	arguments = append(arguments, destination)

	cmd := exec.CommandContext(ctx, "rsync", arguments...)

	if rsyncNode.Spec.UserId != uint32(os.Getuid()) || rsyncNode.Spec.GroupId != uint32(os.Getgid()) {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: rsyncNode.Spec.UserId,
				Gid: rsyncNode.Spec.GroupId,
			},
		}
	}
	log.Info("Rsync credential", "uid", rsyncNode.Spec.UserId, "gid", rsyncNode.Spec.GroupId)

	out, err := cmd.Output()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Info("Rsync failure", "error", string(exitErr.Stderr))
		} else {
			log.Info("Rsync failure", "error", err, "output", string(out))
		}
	} else {
		log.Info("rsync completed", "output", string(out))
	}

	// Record the completion status.
	rsyncNode.Status.EndTime = metav1.NowMicro()
	rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished

	if err != nil {
		rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonFailed
		rsyncNode.Status.Message = err.Error()
	} else {
		rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess
	}

	if err := r.Status().Update(ctx, rsyncNode); err != nil {
		log.Error(err, "failed to update rsync status with completion")
		r.recordCompletion(*rsyncNode)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RsyncNodeDataMovementReconciler) recordCompletion(rsyncNode dmv1alpha1.RsyncNodeDataMovement) {
	r.completions.Store(rsyncNode.Name, rsyncNode)
}

func (r *RsyncNodeDataMovementReconciler) loadCompletion(name string) (dmv1alpha1.RsyncNodeDataMovement, bool) {
	rsyncNode, found := r.completions.Load(name)
	if found {
		return rsyncNode.(dmv1alpha1.RsyncNodeDataMovement), found
	}
	return dmv1alpha1.RsyncNodeDataMovement{}, false
}

func (r *RsyncNodeDataMovementReconciler) deleteCompletion(name string) {
	r.completions.Delete(name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RsyncNodeDataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncNodeDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 128}).
		Complete(r)
}
