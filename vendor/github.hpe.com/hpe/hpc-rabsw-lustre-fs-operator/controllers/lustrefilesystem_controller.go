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

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	driver "github.hpe.com/hpe/hpc-rabsw-lustre-csi-driver/pkg/lustre-driver/service"
	"github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
)

const (
	PersistentVolumeSuffix      = "-pv"
	PersistentVolumeClaimSuffix = "-pvc"
)

var (
	// Capacity, or Storage Resource Quantity, is required parameter and must be non-zero. This value is programmed into both the
	// Persistent Volume and Persistent Volume Claim, but remains unused by any of the Lustre CSI.
	PersistentVolumeResourceQuantity = resource.MustParse("1")
)

// LustreFileSystemReconciler reconciles a LustreFileSystem object
type LustreFileSystemReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;update;create;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;update;create;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LustreFileSystem object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *LustreFileSystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("LustreFileSystem", req.NamespacedName.String())

	log.Info("Starting reconcile loop")
	defer log.Info("Finish reconcile loop")

	fs := &v1alpha1.LustreFileSystem{}
	if err := r.Get(ctx, req.NamespacedName, fs); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO: Check if FS is being deleted

	// TODO: Check if FS has Finalizer; Do we need to add a finalizer to delete the object chain?

	ownerRef := metav1.NewControllerRef(
		&fs.ObjectMeta,
		schema.GroupVersionKind{
			Group:   v1alpha1.GroupVersion.Group,
			Version: v1alpha1.GroupVersion.Version,
			Kind:    "LustreFileSystem",
		})

	for i, fn := range []createOrUpdateFn{createOrUpdatePersistentVolume, createOrUpdatePersistentVolumeClaim} {
		result, err := fn(ctx, r, fs, ownerRef, log)
		if err != nil {
			log.Error(err, "create or update failed", "index", i, "result", result)
			return ctrl.Result{}, err
		}

		log.Info("Create Or Update Object", "index", i, "result", result)
		if result != controllerutil.OperationResultNone {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

type createOrUpdateFn func(context.Context, *LustreFileSystemReconciler, *v1alpha1.LustreFileSystem, *metav1.OwnerReference, logr.Logger) (controllerutil.OperationResult, error)

func createOrUpdatePersistentVolume(ctx context.Context, r *LustreFileSystemReconciler, fs *v1alpha1.LustreFileSystem, ownerRef *metav1.OwnerReference, log logr.Logger) (controllerutil.OperationResult, error) {
	log = log.WithValues("PersistentVolume", fs.Name+PersistentVolumeSuffix)

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: fs.Name + PersistentVolumeSuffix,

			// TODO: For some reason when the namespace is "default", a GET on the PersistentVolume returns an empty namespace string
			//       This causes the modification of the ObjectMeta.Namespace parameter, which is illegal form of a mutate function.
			//       This needs to be investigated further - why does the get not return "default"? Maybe it's because Persistent
			//       Volumes are cluster scoped, and so the namespace is dropped from the return of GET? I'm very confused by this.
			//Namespace: fs.Namespace,
		},
	}

	mutateFn := func() error {

		// WARNING: A cluster-scoped resource like Persistent Volume must not have a namespace-scoped owner (LustreFileSystem),
		//          even if that namespace is "default". So even though this is a controller reference, we are stuck supplying
		//          just the owner reference.
		//if err := ctrl.SetControllerReference(fs, pv, r.Scheme); err != nil {
		//	return err
		//}

		pv.ObjectMeta.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})

		pv.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteMany,
		}

		pv.Spec.Capacity = corev1.ResourceList{
			corev1.ResourceStorage: PersistentVolumeResourceQuantity,
		}

		pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			CSI: &corev1.CSIPersistentVolumeSource{
				Driver:       driver.Name,
				FSType:       "lustre",
				VolumeHandle: fs.Spec.MgsNid + ":/" + fs.Spec.Name,
			},
		}

		volumeMode := corev1.PersistentVolumeFilesystem
		pv.Spec.VolumeMode = &volumeMode

		return nil
	}

	return ctrl.CreateOrUpdate(ctx, r.Client, pv, mutateFn)
}

func createOrUpdatePersistentVolumeClaim(ctx context.Context, r *LustreFileSystemReconciler, fs *v1alpha1.LustreFileSystem, ownerRef *metav1.OwnerReference, log logr.Logger) (controllerutil.OperationResult, error) {
	log = log.WithValues("PersistentVolumeClaim", fs.Name+PersistentVolumeClaimSuffix)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fs.Name + PersistentVolumeClaimSuffix,
			Namespace: fs.Namespace,
		},
	}

	mutateFn := func() error {
		pvc.ObjectMeta.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})

		pvc.Spec.VolumeName = fs.Name + PersistentVolumeSuffix

		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteMany,
		}

		pvc.Spec.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: PersistentVolumeResourceQuantity,
			},
		}

		return nil
	}

	return ctrl.CreateOrUpdate(ctx, r.Client, pvc, mutateFn)
}

// SetupWithManager sets up the controller with the Manager.
func (r *LustreFileSystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LustreFileSystem{}).
		Complete(r)
}
