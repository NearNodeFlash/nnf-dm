/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dwsv1alpha1 "github.hpe.com/hpe/hpc-dpm-dws-operator/api/v1alpha1"
	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

// RsyncNodeDataMovementReconciler reconciles a RsyncNodeDataMovement object
type RsyncNodeDataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	if rsyncNode.Status.StartTime.IsZero() {

		rsyncNode.Status.StartTime = metav1.Now()
		rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning
		if err := r.Status().Update(ctx, rsyncNode); err != nil {
			return ctrl.Result{}, err
		}

		arguments := []string{}
		if rsyncNode.Spec.DryRun {
			arguments = append(arguments, "--dry-run")
		}

		// Start the rsync operation
		source, err := r.getSourcePath(ctx, rsyncNode)
		if err != nil {
			rsyncNode.Status.EndTime = metav1.Now()
			rsyncNode.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
			rsyncNode.Status.Status = nnfv1alpha1.DataMovementConditionReasonInvalid
			if err := r.Status().Update(ctx, rsyncNode); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		destination := rsyncNode.Spec.Destination
		log.V(1).Info("Executing rsync command", "source", source, "destination", destination)

		arguments = append(arguments, source)
		arguments = append(arguments, destination)
		out, err := exec.CommandContext(ctx, "rsync", arguments...).Output()

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				log.V(1).Info("Rsync failure", "error", string(exitErr.Stderr))
			}
		} else {
			log.V(1).Info("rsync completed", "output", string(out))
		}

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
			return ctrl.Result{}, err
		}
	} else if rsyncNode.Status.EndTime.IsZero() {
		log.V(1).Info("Rsync may be running...")

		// Problem here is the rsync could still be running _or_ it could have completed
		// but the EndTime was never recorded. I'm not sure how to solve for this condition.
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

	relativePath := ""
	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if strings.HasPrefix(rsync.Spec.Source, mount.MountPath) {
				relativePath = strings.TrimPrefix(rsync.Spec.Source, mount.MountPath)
				break
			}
		}
	}

	if len(relativePath) == 0 {
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
			if mount.Compute == rsync.Spec.Initiator {
				return mount.MountPath + relativePath, nil
			}
		}
	}

	return "", fmt.Errorf("Initiator '%s' not found in list of client mounts", rsync.Spec.Initiator)

}

// SetupWithManager sets up the controller with the Manager.
func (r *RsyncNodeDataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncNodeDataMovement{}).
		Complete(r)
}
