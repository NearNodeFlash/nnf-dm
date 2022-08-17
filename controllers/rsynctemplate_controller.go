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

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile" // Required for Watching
	"sigs.k8s.io/controller-runtime/pkg/source"

	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"

	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	"github.com/NearNodeFlash/nnf-dm/controllers/metrics"
)

const (
	PersistentVolumeSuffix      = "-pv"
	PersistentVolumeClaimSuffix = "-pvc"

	defaultNnfVolumeName = "mnt-nnf"
)

// RsyncTemplateReconciler reconciles a RsyncTemplate object
type RsyncTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsynctemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsynctemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dm.cray.hpe.com,resources=rsynctemplates/finalizers,verbs=update
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RsyncTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RsyncTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	metrics.NnfDmRsyncTemplateReconcilesTotal.Inc()

	rsyncTemplate := &dmv1alpha1.RsyncTemplate{}
	if err := r.Get(ctx, req.NamespacedName, rsyncTemplate); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	enableLustreFileSystems := rsyncTemplate.Spec.DisableLustreFileSystems == nil || *(rsyncTemplate.Spec.DisableLustreFileSystems) == false

	filesystems := &lusv1alpha1.LustreFileSystemList{}
	if enableLustreFileSystems {
		if err := r.List(ctx, filesystems); err != nil {
			return ctrl.Result{}, err
		}
	}

	ds := &apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nnf-dm-rsyncnode",
			Namespace: rsyncTemplate.Namespace,
		},
	}

	mutateFn := func() error {

		if err := ctrl.SetControllerReference(rsyncTemplate, ds, r.Scheme); err != nil {
			return err
		}

		t := &rsyncTemplate.Spec.Template

		// Always match the nodes with the selector labels
		t.ObjectMeta.Labels = rsyncTemplate.Spec.Selector.MatchLabels
		t.Spec.NodeSelector = rsyncTemplate.Spec.Selector.MatchLabels

		// Add the volumes and volume mounts to the specification
		t.Spec.Volumes = append(t.Spec.Volumes, volumes(rsyncTemplate, filesystems.Items)...)

		for cidx := range t.Spec.Containers {
			container := &t.Spec.Containers[cidx]
			container.VolumeMounts = append(container.VolumeMounts, volumeMounts(rsyncTemplate, filesystems.Items)...)
		}

		ds.Spec = apps.DaemonSetSpec{
			Selector: &rsyncTemplate.Spec.Selector,
			Template: *t,
		}

		return nil
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, ds, mutateFn)
	if err != nil {
		log.Error(err, "failed to create or update daemonset", "name", ds.Name, "namespace", ds.Namespace)
		return ctrl.Result{}, err
	}

	if result == controllerutil.OperationResultCreated {
		log.Info("Created DaemonSet", "name", ds.Name, "namespace", ds.Namespace, "enableLustreFS", enableLustreFileSystems, "filesystems found", len(filesystems.Items))
	} else if result == controllerutil.OperationResultNone {
		// no change
	} else {
		log.Info("Updated DaemonSet", "name", ds.Name, "namespace", ds.Namespace, "enableLustreFS", enableLustreFileSystems, "filesystems found", len(filesystems.Items))
	}

	return ctrl.Result{}, nil
}

func volumes(rsync *dmv1alpha1.RsyncTemplate, filesystems []lusv1alpha1.LustreFileSystem) []core.Volume {
	vols := make([]core.Volume, len(filesystems))
	for idx, fs := range filesystems {
		vols[idx] = core.Volume{
			Name: fs.Name,
			VolumeSource: core.VolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: fs.Name + PersistentVolumeClaimSuffix,
				},
			},
		}
	}

	hostPathType := core.HostPathDirectoryOrCreate
	vols = append(vols, core.Volume{
		Name: defaultNnfVolumeName,
		VolumeSource: core.VolumeSource{
			HostPath: &core.HostPathVolumeSource{
				Path: rsync.Spec.HostPath,
				Type: &hostPathType,
			},
		},
	})

	return vols
}

func volumeMounts(rsync *dmv1alpha1.RsyncTemplate, filesystems []lusv1alpha1.LustreFileSystem) []core.VolumeMount {
	mounts := make([]core.VolumeMount, len(filesystems))
	for idx, fs := range filesystems {
		mounts[idx] = core.VolumeMount{
			Name:      fs.Name,
			MountPath: fs.Spec.MountRoot,
		}
	}

	mountPropagation := core.MountPropagationHostToContainer
	mounts = append(mounts, core.VolumeMount{
		Name:             defaultNnfVolumeName,
		MountPath:        rsync.Spec.MountPath,
		MountPropagation: &mountPropagation,
	})

	return mounts
}

// Enqueue requests for all Rsync Templates
func (r *RsyncTemplateReconciler) updateRsyncTemplates(client.Object) []reconcile.Request {

	templates := &dmv1alpha1.RsyncTemplateList{}
	if err := r.List(context.TODO(), templates); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(templates.Items))
	for idx, item := range templates.Items {
		requests[idx] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *RsyncTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncTemplate{}).
		Owns(&apps.DaemonSet{}).
		Watches(
			&source.Kind{Type: &lusv1alpha1.LustreFileSystem{}},
			handler.EnqueueRequestsFromMapFunc(r.updateRsyncTemplates),
		).
		Complete(r)
}
