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
	"strings"

	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/HewlettPackard/lustre-csi-driver/pkg/lustre-driver/service"
	"github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
)

const (
	PersistentVolumeSuffix      = "-pv"
	PersistentVolumeClaimSuffix = "-pvc"

	finalizerFS = "cray.hpe.com/lustre_fs_operator"
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
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;update;create;patch;delete;watch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;update;create;patch;delete;watch

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

	fs := &v1alpha1.LustreFileSystem{}
	if err := r.Get(ctx, req.NamespacedName, fs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Starting reconcile loop")
	defer log.Info("Finish reconcile loop")

	// Check if the object is being deleted.
	if !fs.GetDeletionTimestamp().IsZero() {

		if !controllerutil.ContainsFinalizer(fs, finalizerFS) {
			return ctrl.Result{}, nil
		}

		// Delete the PV, if it still exists.  The PVC will be
		// cleaned up by garbage collection.
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: fs.Name + PersistentVolumeSuffix,
			},
		}
		if err := r.Delete(ctx, pv); err != nil {
			log.Error(err, "Unable to delete PV", "pv", pv.GetName())
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		} else {
			log.Info("Deleted PV", "pv", pv.GetName())
		}

		controllerutil.RemoveFinalizer(fs, finalizerFS)
		if err := r.Update(ctx, fs); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(fs, finalizerFS) {
		controllerutil.AddFinalizer(fs, finalizerFS)
		if err := r.Update(ctx, fs); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	}

	ownerRef := metav1.NewControllerRef(
		&fs.ObjectMeta,
		schema.GroupVersionKind{
			Group:   v1alpha1.GroupVersion.Group,
			Version: v1alpha1.GroupVersion.Version,
			Kind:    "LustreFileSystem",
		})

	for i, fn := range []createOrUpdateFn{createOrUpdatePersistentVolumeClaim, createOrUpdatePersistentVolume} {
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
		},
	}

	mutateFn := func() error {

		// A cluster-scoped resource like Persistent Volume must not
		// have a namespace-scoped owner (LustreFileSystem), even if
		// that namespace is "default".  A 'describe' of the resource
		// will show warnings, if an ownerRef has been set.

		pv.Spec.StorageClassName = fs.Spec.StorageClassName
		pv.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteMany,
		}

		pv.Spec.Capacity = corev1.ResourceList{
			corev1.ResourceStorage: PersistentVolumeResourceQuantity,
		}

		if pv.Spec.ClaimRef != nil && pv.Status.Phase == corev1.VolumeReleased {
			// The PV is being updated, and it was bound to a PVC
			// but now it's released.  Clear the uid of the
			// earlier PVC from the claimRef.
			// This allows it to bind to a new PVC.
			pv.Spec.ClaimRef.UID = ""
		}
		if pv.Spec.ClaimRef == nil {
			pv.Spec.ClaimRef = &corev1.ObjectReference{}
		}
		// Reserve this PV for the matching PVC.
		pv.Spec.ClaimRef.Name = fs.Name + PersistentVolumeClaimSuffix
		pv.Spec.ClaimRef.Namespace = fs.Namespace

		pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			CSI: &corev1.CSIPersistentVolumeSource{
				Driver:       service.Name,
				FSType:       "lustre",
				VolumeHandle: strings.Join(fs.Spec.MgsNids, ":") + ":/" + fs.Spec.Name,
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

		pvc.Spec.StorageClassName = &fs.Spec.StorageClassName
		// Reserve this PVC for the matching PV.
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
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
