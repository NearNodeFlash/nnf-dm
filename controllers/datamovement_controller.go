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
	"reflect"

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

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"
)

const (
	finalizer = "dm.cray.hpe.com"
	
	InitiatorLabel = "dm.cray.hpe.com/initiator"
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsyncnodedatamovements,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfaccesses,verbs=get;list;watch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubeflow.org,resources=mpijobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubeflow.org,resources=mpijobs/status,verbs=get;update;patch
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

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the object is being deleted. Deletion is coordinated around the sub-resources
	// created or modified as part of data movement.
	if !dm.GetDeletionTimestamp().IsZero() {
		log.Info("Starting delete operation")

		if !controllerutil.ContainsFinalizer(dm, finalizer) {
			return ctrl.Result{}, nil
		}

		isLustre2Lustre, _ := r.isLustre2Lustre(ctx, dm)
		teardownFn := map[bool]func(context.Context, *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error){
			false: r.teardownRsyncJob,
			true:  r.teardownLustreJob,
		}[isLustre2Lustre]

		result, err := teardownFn(ctx, dm)
		log.Info("Teardown", "Result", result, "Error", err)
		if err != nil {
			return ctrl.Result{}, err
		} else if result != nil {
			return *result, nil
		}

		controllerutil.RemoveFinalizer(dm, finalizer)
		if err := r.Update(ctx, dm); err != nil {
			log.Error(err, "Failed to update after removing finalizer")
			return ctrl.Result{}, err
		}

		log.Info("Successfully deleted data movement resource")
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dm, finalizer) {

		// Do first level validation
		if len(dm.Status.Conditions) == 0 {

			if err := r.validateSpec(dm); err != nil {
				advanceCondition(dm,
					nnfv1alpha1.DataMovementConditionTypeFinished,
					nnfv1alpha1.DataMovementConditionReasonInvalid,
					fmt.Sprintf("Input validation failed: %v", err),
				)
			} else {
				advanceCondition(dm,
					nnfv1alpha1.DataMovementConditionTypeStarting,
					nnfv1alpha1.DataMovementConditionReasonSuccess,
					"Data movement resource starting",
				)
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

	isLustre2Lustre, err := r.isLustre2Lustre(ctx, dm)
	if err != nil {
		return ctrl.Result{}, err
	}

	currentConditionType := dm.Status.Conditions[len(dm.Status.Conditions)-1].Type
	log.Info("Executing", "IsLustre2Lustre", isLustre2Lustre, "Condition", currentConditionType)
	switch currentConditionType {

	case nnfv1alpha1.DataMovementConditionTypeStarting:

		startFn := map[bool]func(context.Context, *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error){
			false: r.initializeRsyncJob,
			true:  r.initializeLustreJob,
		}[isLustre2Lustre]

		result, err := startFn(ctx, dm)
		if err != nil {
			log.Error(err, "Failed to start")
			return ctrl.Result{}, err
		} else if result != nil && !result.IsZero() {
			return *result, nil
		}

		advanceCondition(dm,
			nnfv1alpha1.DataMovementConditionTypeRunning,
			nnfv1alpha1.DataMovementConditionReasonSuccess,
			"Data movement resource running",
		)

		if err := r.Status().Update(ctx, dm); err != nil {
			log.Error(err, "Failed to transition to running state")
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil

	case nnfv1alpha1.DataMovementConditionTypeRunning:
		monitorFn := map[bool]func(context.Context, *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, string, string, error){
			false: r.monitorRsyncJob,
			true:  r.monitorLustreJob,
		}[isLustre2Lustre]

		result, status, message, err := monitorFn(ctx, dm)
		log.Info("Monitor job", "status", status, "message", message)

		if err != nil {
			log.Error(err, "Failed to monitor")
			return ctrl.Result{}, err
		} else if result != nil && !result.IsZero() {
			return *result, nil
		}

		// Continue monitoring the resource, if specified, by preventing the data movement resource from finishing
		// early while the client application is running and generating data movement requests. This is to protect
		// the use case where this resource thinks it handled all the requests but in reality the client application
		// continues to execute with the potential to generate more requests.
		if dm.Spec.Monitor == true {
			return ctrl.Result{}, nil
		}

		switch status {
		case nnfv1alpha1.DataMovementConditionTypeRunning:
			// Still running, nothing to do here
			break
		case nnfv1alpha1.DataMovementConditionReasonFailed, nnfv1alpha1.DataMovementConditionReasonSuccess:

			advanceCondition(dm,
				nnfv1alpha1.DataMovementConditionTypeFinished,
				status,
				message,
			)

		}

		if err := r.Status().Update(ctx, dm); err != nil {
			log.Error(err, "Failed to transition to finished state")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil

	case nnfv1alpha1.DataMovementConditionTypeFinished:
		return ctrl.Result{}, nil // Already finished, do nothing further.
	}

	return ctrl.Result{}, nil
}

func (r *DataMovementReconciler) validateSpec(dm *nnfv1alpha1.NnfDataMovement) error {

	// Permit empty data movement resources; this results in a data movement resource
	// that monitors existing rsync resources
	if dm.Spec.Source == nil && dm.Spec.Destination == nil {
		return nil
	}

	if dm.Spec.Source == nil || len(dm.Spec.Source.Path) == 0 {
		return fmt.Errorf("source path must be defined")
	}
	if dm.Spec.Destination == nil || len(dm.Spec.Destination.Path) == 0 {
		return fmt.Errorf("destination path must be defined")
	}

	// If destination is just "path" this must be a lustre file system
	if dm.Spec.Source.Storage == nil && dm.Spec.Destination.Storage == nil {
		return fmt.Errorf("one of source or destination must be a storage instance")
	}

	return nil
}

func (r *DataMovementReconciler) isLustre2Lustre(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (isLustre bool, err error) {
	if dm.Spec.Source == nil || dm.Spec.Destination == nil {
		return false, nil
	}

	// Data Movement is a Lustre2Lustre copy if...
	//   COPY_IN and Source is LustreFileSystem and
	//      Destination is NnfStorage.fsType == lustre or
	//      Destination is PersistentStorageInstance.fsType == lustre
	//   or
	//   COPY_OUT and Destination is LustreFileSystem and
	//      Source is NnfStorage.fsType == lustre or
	//      Source is PersistentStorageInstance.fsType == lustre
	fsType := ""
	if dm.Spec.Source.Storage != nil && dm.Spec.Source.Storage.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {
		fsType, err = r.getStorageInstanceFileSystemType(ctx, dm.Spec.Destination.Storage)
	} else if dm.Spec.Destination.Storage != nil && dm.Spec.Destination.Storage.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {
		fsType, err = r.getStorageInstanceFileSystemType(ctx, dm.Spec.Source.Storage)
	}

	return fsType == "lustre", err
}

func advanceCondition(dm *nnfv1alpha1.NnfDataMovement, typ string, reason string, message string) {
	if len(dm.Status.Conditions) != 0 {
		dm.Status.Conditions[len(dm.Status.Conditions)-1].Status = metav1.ConditionFalse
	}

	dm.Status.Conditions = append(dm.Status.Conditions, metav1.Condition{
		Type:               typ,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		Status:             metav1.ConditionTrue,
	})
}

func (r *DataMovementReconciler) getStorageInstanceFileSystemType(ctx context.Context, object *corev1.ObjectReference) (string, error) {

	switch object.Kind {
	case reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name():

		storage := &nnfv1alpha1.NnfStorage{}
		if err := r.Get(ctx, types.NamespacedName{Name: object.Name, Namespace: object.Namespace}, storage); err != nil {
			return "", err
		}

		return storage.Spec.FileSystemType, nil

	case reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name():
		return "lustre", nil
	}

	panic(fmt.Sprintf("Unsupported storage type '%s'", object.Kind))
}

func (r *DataMovementReconciler) getDataMovementConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{}

	if err := r.Get(ctx, types.NamespacedName{Name: "data-movement" + configSuffix, Namespace: configNamespace}, config); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	return config, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovement{}).
		Owns(&mpiv2beta1.MPIJob{}).
		Owns(&corev1.PersistentVolume{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(
			&source.Kind{Type: &mpiv2beta1.MPIJob{}},
			handler.EnqueueRequestsFromMapFunc(mpijobEnqueueRequestMapFunc),
		).
		Watches(
			&source.Kind{Type: &dmv1alpha1.RsyncNodeDataMovement{}},
			handler.EnqueueRequestsFromMapFunc(dwsv1alpha1.OwnerLabelMapFunc),
		).
		Complete(r)
}
