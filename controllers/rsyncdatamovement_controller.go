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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

// RsyncDataMovementReconciler reconciles a RsyncDataMovement object
type RsyncDataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncdatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncdatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncdatamovements/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RsyncDataMovement object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RsyncDataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("Rsync", req.NamespacedName.String())

	rsync := &dmv1alpha1.RsyncDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, rsync); err != nil {
		log.Error(err, "Failed to get RsyncDataMovememnt")
		return ctrl.Result{}, err
	}

	// Retrieve the NnfStorage object that is associated with this Rsync job. This provides the list of Rabbits that will receive
	// RsyncNodeDataMovement resources. TODO: Add rbac policy to perfrom GET
	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: rsync.Spec.Storage.Name, Namespace: rsync.Spec.Storage.Namespace}, storage); err != nil {
		log.Error(err, "Failed to get NnfStorage")
		return ctrl.Result{}, err
	}

	// TODO: Retrieve the NnfNodeLocalAccess object that is associated with this Rsync job. Should then retrieve the list of computes
	//       associated with the node local access, which is used to start the data movement

	// The NnfStorage specification is a list of Allocation Sets; with each set containing a number of Nodes <Name, Count> pair that
	// describes the Rabbit and the number of allocations to perform on that node. Since this is an Rsync job, the expected number
	// of Allocation Sets is one, and it should be of xfs/gfs2 type.
	for _, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.FileSystemType == "xfs" || allocationSet.FileSystemType == "gfs2" {
			// TODO: Iterate over the list of Nodes in this allocation set, create an RsyncNodeDataMovement object that
			// 1. Name is the rsync job name + compute ID
			// 2. Namespace is the Rabbit name
			// 3. Spec.Source is the Rsync Source
			// 4. Spec.Destination is the NodeLocalAccess path + compute ID + Rsync Destination

			// TODO: Compute ID is found by filtering the list of Computes for the target Rabbit; this algorithm must use a solid naming
			//       convention (x-names or something similiar) so we can get the computes associated with the target Rabbit.
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RsyncDataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncDataMovement{}).
		Owns(&dmv1alpha1.RsyncNodeDataMovement{}).
		Complete(r)
}
