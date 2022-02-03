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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"
	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

const (
	finalizer = "dm.cray.hpe.com"
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=datamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfjobstorageinstances,verbs=get;list;watch
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubeflow.org,resources=mpijobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;pathc;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update

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
	log := log.FromContext(ctx)

	dm := &dmv1alpha1.DataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the object is being deleted. Deletion is coordinated around the sub-resources
	// created or modified as part of data movement.
	if !dm.GetDeletionTimestamp().IsZero() {
		log.V(2).Info("Starting delete operation")

		if !controllerutil.ContainsFinalizer(dm, finalizer) {
			return ctrl.Result{}, nil
		}

		isLustre2Lustre, _ := r.isLustre2Lustre(dm)
		teardownFn := map[bool]func(context.Context, *dmv1alpha1.DataMovement) (*ctrl.Result, error){
			false: r.teardownRsyncJob,
			true:  r.teardownLustreJob,
		}

		result, err := teardownFn[isLustre2Lustre](ctx, dm)
		log.V(2).Info("Teardown", "Result", result, "Error", err)
		if err != nil {
			return ctrl.Result{}, err
		} else if !result.IsZero() {
			return *result, nil
		}

		controllerutil.RemoveFinalizer(dm, finalizer)
		if err := r.Update(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dm, finalizer) {

		// Do first level validation
		if len(dm.Status.Conditions) == 0 {

			if err := r.validateSpec(dm); err != nil {
				dm.Status.Conditions = []metav1.Condition{{
					Type:               dmv1alpha1.DataMovementConditionTypeFinished,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             dmv1alpha1.DataMovementConditionReasonInvalid,
					Message:            fmt.Sprintf("Input validation failed: %v", err),
				}}
			} else {
				dm.Status.Conditions = []metav1.Condition{{
					Type:               dmv1alpha1.DataMovementConditionTypeStarting,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             dmv1alpha1.DataMovementConditionReasonSuccess,
					Message:            "Data movement resource starting",
				}}
			}

			if err := r.Status().Update(ctx, dm); err != nil {
				log.Error(err, "Failed to initialize status")
				if errors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}

				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}

		controllerutil.AddFinalizer(dm, finalizer)
		if err := r.Update(ctx, dm); err != nil {
			log.Error(err, "Failed to initialize finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	isLustre2Lustre, err := r.isLustre2Lustre(dm)
	if err != nil {
		return ctrl.Result{}, err
	}

	currentConditionType := dm.Status.Conditions[len(dm.Status.Conditions)-1].Type
	log.V(1).Info("Executing", "IsLustre", isLustre2Lustre, "Condition", currentConditionType)
	switch currentConditionType {

	case dmv1alpha1.DataMovementConditionTypeStarting:

		startFn := map[bool]func(context.Context, *dmv1alpha1.DataMovement) (*ctrl.Result, error){
			false: r.initializeRsyncJob,
			true:  r.initializeLustreJob,
		}[isLustre2Lustre]

		result, err := startFn(ctx, dm)
		if err != nil {
			log.Error(err, "Failed to start")
			return ctrl.Result{}, err
		} else if !result.IsZero() {
			return *result, nil
		}

		dm.Status.Conditions = append(dm.Status.Conditions, metav1.Condition{
			Type:               dmv1alpha1.DataMovementConditionTypeRunning,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Message:            "Data movement resource running",
			Reason:             "ResourceRunning",
		})

		if err := r.Status().Update(ctx, dm); err != nil {
			log.Error(err, "Failed to transition to running state")
			return ctrl.Result{}, err
		}

		log.Info("Data Movement Running")
		return ctrl.Result{Requeue: true}, nil

	case dmv1alpha1.DataMovementConditionTypeRunning:
		monitorFn := map[bool]func(context.Context, *dmv1alpha1.DataMovement) (*ctrl.Result, string, string, error){
			false: r.monitorRsyncJob,
			true:  r.monitorLustreJob,
		}[isLustre2Lustre]

		result, status, message, err := monitorFn(ctx, dm)
		if err != nil {
			log.Error(err, "Failed to monitor")
			return ctrl.Result{}, err
		} else if !result.IsZero() {
			return *result, nil
		}

		switch status {
		case dmv1alpha1.DataMovementConditionTypeRunning:
			// Still running, nothing to do here
			break
		case dmv1alpha1.DataMovementConditionReasonFailed, dmv1alpha1.DataMovementConditionReasonSuccess:

			dm.Status.Conditions[len(dm.Status.Conditions)-1].Status = metav1.ConditionFalse

			// Note: In this case status == reason, so we can use it directly in the condition below
			dm.Status.Conditions = append(dm.Status.Conditions, metav1.Condition{
				Type:               dmv1alpha1.DataMovementConditionTypeFinished,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Message:            message,
				Reason:             status,
			})
		}

		if err := r.Status().Update(ctx, dm); err != nil {
			log.Error(err, "Failed to transition to finished state")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil

	case dmv1alpha1.DataMovementConditionTypeFinished:
		return ctrl.Result{}, nil // Already finished, do nothing further.
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
	// Data Movement is a Lustre2Lustre copy if...
	//   COPY_IN and Source is LustreFileSystem and
	//      Destination is JobStorageInstance.fsType == lustre or
	//      Destination is PersistentStorageInstance.fsType == lustre
	//   or
	//   COPY_OUT and Destination is LustreFileSystem and
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
	case "NnfJobStorageInstance":

		jobStorageInstance := &nnfv1alpha1.NnfJobStorageInstance{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: object.Name, Namespace: object.Namespace}, jobStorageInstance); err != nil {
			return "", err
		}

		return jobStorageInstance.Spec.FsType, nil

	case "NnfPersistentStorageInstance":
	}

	return "lustre", nil
}

func (r *DataMovementReconciler) getDataMovementConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{}

	// TODO: This should move to the Data Movement Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: "data-movement" + configSuffix, Namespace: corev1.NamespaceDefault}, config); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	return config, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.DataMovement{}).
		Owns(&mpiv2beta1.MPIJob{}).
		Owns(&corev1.PersistentVolume{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(
			&source.Kind{Type: &dmv1alpha1.RsyncNodeDataMovement{}},
			handler.EnqueueRequestsFromMapFunc(rsyncNodeDataMovementEnqueueRequestMapFunc),
		).
		Complete(r)
}
