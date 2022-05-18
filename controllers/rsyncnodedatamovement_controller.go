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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
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
	rsyncNode.Status.StartTime = metav1.Now()
	rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning
	if err := r.Status().Update(ctx, rsyncNode); err != nil {
		log.Error(err, "failed to set rsync node as running")
		return ctrl.Result{}, err
	}

	arguments := []string{}
	if rsyncNode.Spec.DryRun {
		arguments = append(arguments, "--dry-run")
	}

	// Find the source path for this request. This could be a translation from the compute-local
	// path (if initiator is a compute node); otherwise it is the rabbit-local path.
	source, err := r.getSourcePath(ctx, rsyncNode)
	if err != nil {
		rsyncNode.Status.EndTime = metav1.Now()
		rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
		rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonInvalid
		rsyncNode.Status.Message = err.Error()
		if err := r.Status().Update(ctx, rsyncNode); err != nil {
			log.Error(err, "failed to set rsync node as invalid")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	destination := rsyncNode.Spec.Destination
	log.V(1).Info("Executing rsync command", "source", source, "destination", destination)

	arguments = append(arguments, "--recursive")
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

	out, err := cmd.Output()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.V(1).Info("Rsync failure", "error", string(exitErr.Stderr))
		} else {
			log.V(1).Info("Rsync failure", "error", err)
		}
	} else {
		log.V(1).Info("rsync completed", "output", string(out))
	}

	// Record the completion status.
	rsyncNode.Status.EndTime = metav1.Now()
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

// Return the source path given the rsync node data movement request. This takes a compute-local path (the initiator) and returns
// the the rabbit-relative path. If no initiator is provided then the source path is assumed to already be local to the rabbit.
func (r *RsyncNodeDataMovementReconciler) getSourcePath(ctx context.Context, rsync *dmv1alpha1.RsyncNodeDataMovement) (string, error) {
	if len(rsync.Spec.Initiator) == 0 {
		return rsync.Spec.Source, nil
	}

	// Look up the client mounts on the initiator to find the compute relative mount path. The "spec.Source" must be
	// prefixed with a mount path in the list of mounts. Once we find this mount, we can strip out the prefix and
	// are left with the relative path.

	clientMounts := &dwsv1alpha1.ClientMountList{}
	listOptions := []client.ListOption{
		client.InNamespace(rsync.Spec.Initiator),
		client.MatchingLabels(map[string]string{
			dwsv1alpha1.WorkflowNameLabel:      rsync.Labels[dmv1alpha1.OwnerLabelRsyncNodeDataMovement],
			dwsv1alpha1.WorkflowNamespaceLabel: rsync.Labels[dmv1alpha1.OwnerNamespaceLabelRsyncNodeDataMovement],
		}),
	}

	if err := r.List(ctx, clientMounts, listOptions...); err != nil {
		return "", err
	}

	computeMountInfo := dwsv1alpha1.ClientMountInfo{}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if strings.HasPrefix(rsync.Spec.Source, mount.MountPath) {
				if mount.Device.DeviceReference == nil {
					return "", fmt.Errorf("Source '%s' does not have NnfNodeStorage reference in client mount", rsync.Spec.Source)
				}

				computeMountInfo = mount
				break
			}
		}
	}

	if computeMountInfo == (dwsv1alpha1.ClientMountInfo{}) {
		return "", fmt.Errorf("Source '%s' not found in list of client mounts", rsync.Spec.Source)
	}

	// Now look up the client mount on this Rabbit node and find the compute initiator. We append the relative path
	// to this value resulting in the full path on the Rabbit.

	listOptions = []client.ListOption{
		client.InNamespace(rsync.GetNamespace()),
		client.MatchingLabels(map[string]string{
			dwsv1alpha1.WorkflowNameLabel:      rsync.Labels[dmv1alpha1.OwnerLabelRsyncNodeDataMovement],
			dwsv1alpha1.WorkflowNamespaceLabel: rsync.Labels[dmv1alpha1.OwnerNamespaceLabelRsyncNodeDataMovement],
		}),
	}

	if err := r.List(ctx, clientMounts, listOptions...); err != nil {
		return "", err
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if computeMountInfo.Device.DeviceReference == mount.Device.DeviceReference {
				return mount.MountPath + strings.TrimPrefix(rsync.Spec.Source, mount.MountPath), nil
			}
		}
	}

	return "", fmt.Errorf("Initiator '%s' not found in list of client mounts", rsync.Spec.Initiator)
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
		Complete(r)
}
