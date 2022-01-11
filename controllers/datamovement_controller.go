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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataMovement object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *DataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("DataMovement", req.NamespacedName.String())

	log.Info("Starting reconcile")
	defer log.Info("Finished reconcile")

	dm := &dmv1alpha1.DataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, err
	}

	if len(dm.Status.Conditions) == 0 {

		if err := r.validateSpec(dm); err != nil {
			dm.Status.Conditions = []metav1.Condition{{
				Type:               dmv1alpha1.DataMovementConditionFinished,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Message:            fmt.Sprintf("input validation failed: %v", err),
			}}
		} else {
			dm.Status.Conditions = []metav1.Condition{{
				Type:               dmv1alpha1.DataMovementConditionCreating,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Message:            "data movement resource creating",
			}}
		}

		if err := r.Status().Update(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	lastConditionType := dm.Status.Conditions[len(dm.Status.Conditions)-1].Type
	switch lastConditionType {

	case dmv1alpha1.DataMovementConditionFinished:
		// Already finished, do nothing further.
		return ctrl.Result{}, nil

	case dmv1alpha1.DataMovementConditionCreating:
		// TODO: Transition to running

		// LUSTRE 2 LUSTRE
		// We need to label all the nodes in the Servers object with a unique label that describes this
		// data movememnt. This label is then used as a selector within the the MPIJob so it correctly
		// targets all the nodes

		// XFS / GFS2
		// We need to forward the NnfNodeAccess, Servers, and Source and Destination paths to the Rsync Job
		// This should be a "CreateOrUpdate" call.

	case dmv1alpha1.DataMovementConditionRunning:
		// TODO: Monitor the underlying MPIJob or RsyncJob for progress; if either of them transition to
		// a Finished state, update the status here and transition myself to Finished.
	}

	return ctrl.Result{}, nil
}

func (r *DataMovementReconciler) validateSpec(dm *dmv1alpha1.DataMovement) error {
	// Validation

	// If source is just "path" this must be a lustre file system
	if len(dm.Spec.Source.Path) == 0 {
		return fmt.Errorf("source path must be defined")
	}
	if len(dm.Spec.Destination.Path) == 0 {
		return fmt.Errorf("destination path must be defined")
	}

	// If destination is just "path" this must be a lustre file system
	if dm.Spec.Source.StorageInstance == nil && dm.Spec.Destination.StorageInstance == nil {
		return fmt.Errorf("one of source or destination must be a storage instance")
	}

	return nil
}

func (r *DataMovementReconciler) isLustre2Lustre(dm *dmv1alpha1.DataMovement) (isLustre bool, err error) {
	// TODO: Data Movement is a Lustre2Lustre copy if
	//   COPYIN and Source is LustreFileSystem and
	//      Destination is JobStorageInstance.fsType == lustre or
	//      Destination is PersistentStorageInstance.fsType == lustre
	//   or
	//   COPYOUT and Destination is LustreFileSystem and
	//      Source is JobStorageInstance.fsType == lustre or
	//      Source is PersistentStorageInstance.fsType == lustre
	fsType := ""
	if dm.Spec.Source.StorageInstance != nil && dm.Spec.Source.StorageInstance.Kind == "LustreFileSystem" {
		fsType, err = r.getStorageInstanceFileSystemType(dm.Spec.Destination.StorageInstance)
	} else if dm.Spec.Destination.StorageInstance != nil && dm.Spec.Destination.StorageInstance.Kind == "LustreFileSystem" {
		fsType, err = r.getStorageInstanceFileSystemType(dm.Spec.Source.StorageInstance)
	}

	return fsType == "lustre", err
}

func (r *DataMovementReconciler) getStorageInstanceFileSystemType(object *corev1.ObjectReference) (string, error) {
	switch object.Kind {
	case "JobStorageInstance":
		jobStorageInstance := &nnfv1alpha1.NnfJobStorageInstance{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: object.Name, Namespace: object.Namespace}, jobStorageInstance); err != nil {
			return "", err
		}

		return jobStorageInstance.Spec.FsType, nil
	case "PersistentStorageInstance":
	}
	return "lustre", nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.DataMovement{}).
		Complete(r)
}
