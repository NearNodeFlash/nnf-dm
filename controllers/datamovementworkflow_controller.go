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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

const (
	workflowFinalizer = "dm.cray.hpe.com"
)

// DataMovementWorkflowReconciler reconciles a NnfDataMovementWorkflow object
type DataMovementWorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementworkflows,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementworkflows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementworkflows/finalizers,verbs=update
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements,verbs=get;list;watch;create;update;patch;delete;deletecollection

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataMovement object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *DataMovementWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	dmw := &nnfv1alpha1.NnfDataMovementWorkflow{}
	if err := r.Get(ctx, req.NamespacedName, dmw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !dmw.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(dmw, workflowFinalizer) {
			return ctrl.Result{}, nil
		}

		result, err := r.DeleteRsyncNodeDataMovements(ctx, dmw)
		if err != nil {
			return ctrl.Result{}, err
		} else if !result.IsZero() {
			return result, nil
		}

		controllerutil.RemoveFinalizer(dmw, workflowFinalizer)
		if err := r.Update(ctx, dmw); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dmw, workflowFinalizer) {
		controllerutil.AddFinalizer(dmw, workflowFinalizer)
		if err := r.Update(ctx, dmw); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DataMovementWorkflowReconciler) DeleteRsyncNodeDataMovements(ctx context.Context, dmw *nnfv1alpha1.NnfDataMovementWorkflow) (ctrl.Result, error) {

	rsyncNodes := &dmv1alpha1.RsyncNodeDataMovementList{}
	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{
			dmv1alpha1.OwnerLabelRsyncNodeDataMovement:          dmw.Name,
			dmv1alpha1.OwnerNamespaceLabelRsyncNodeDataMovement: dmw.Namespace,
		}),
	}
	if err := r.List(ctx, rsyncNodes, listOptions...); err != nil {
		return ctrl.Result{}, err
	}

	if len(rsyncNodes.Items) == 0 {
		return ctrl.Result{}, nil
	}

	for _, rsyncNode := range rsyncNodes.Items {
		if rsyncNode.Status.EndTime.IsZero() {
			continue
		}

		// TODO: We might still be able to delete these if they are in progress because their own finalizer will prevent an
		// ongoing operation from deleting. I prefer the check above so we allow them to complete naturally.
		if err := client.IgnoreNotFound(r.Delete(ctx, &rsyncNode)); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Second}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovementWorkflow{}).
		Complete(r)
}
