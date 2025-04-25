/*
 * Copyright 2022-2025 Hewlett Packard Enterprise Development LP
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

package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"go.openly.dev/pointy"

	"golang.org/x/crypto/ssh"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/keyutil"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataWorkflowServices/dws/utils/updater"
	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	"github.com/NearNodeFlash/nnf-dm/internal/controller/metrics"
	nnfv1alpha7 "github.com/NearNodeFlash/nnf-sos/api/v1alpha7"
)

const (
	deploymentName = "nnf-dm-controller-manager"
	daemonsetName  = "nnf-dm-worker"
	serviceName    = "dm"

	nnfVolumeName = "nnf"

	sshAuthVolume = "ssh-auth"
	sshPublicKey  = "ssh-publickey"
)

var (
	sshAuthVolumeItems = []corev1.KeyToPath{
		{
			Key:  corev1.SSHAuthPrivateKey,
			Path: "id_rsa",
		},
		{
			Key:  sshPublicKey,
			Path: "id_rsa.pub",
		},
		{
			Key:  sshPublicKey,
			Path: "authorized_keys",
		},
	}
)

// NnfDataMovementManagerReconciler reconciles a NnfDataMovementManager object
type NnfDataMovementManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementmanagers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementmanagers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementmanagers/finalizers,verbs=update

// Data Movement Manager initializes the secrets used in establishing SSH connections between the data movement deployment
// and the data movement daemonset describing the worker nodes.
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Data Movement Manager initializes the deployment for controlling Data Movement resources
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Data Movement Manager maintains a Service with the correct subdomain to make nodes reachable via DNS
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Data Movement Manager maintains the DaemonSet with the desired volume mounts for accessing global Lustre file systems
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

// Data Movement Manager watches LustreFileSystems to ensure the volume mounts on the worker nodes are current
//+kubebuilder:rbac:groups=lus.cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NnfDataMovementManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := log.FromContext(ctx)

	metrics.NnfDmDataMovementManagerReconcilesTotal.Inc()

	manager := &nnfv1alpha7.NnfDataMovementManager{}
	if err := r.Get(ctx, req.NamespacedName, manager); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	statusUpdater := updater.NewStatusUpdater[*nnfv1alpha7.NnfDataMovementManagerStatus](manager)
	defer func() { err = statusUpdater.CloseWithStatusUpdate(ctx, r.Client.Status(), err) }()

	errorHandler := func(err error, msg string) (ctrl.Result, error) {
		if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		manager.Status.Ready = false
		log.Error(err, msg+" failed")
		return ctrl.Result{}, err
	}

	if err := r.createSecretIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "create Secret")
	}

	if err := r.createOrUpdateDeploymentIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "create or update Deployment")
	}

	if err := r.createOrUpdateServiceIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "create or update Service")
	}

	if err := r.updateLustreFileSystemsIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "update LustreFileSystems")
	}

	if err := r.createOrUpdateDaemonSetIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "create or update DaemonSet")
	}

	if err := r.removeLustreFileSystemsFinalizersIfNecessary(ctx, manager); err != nil {
		return errorHandler(err, "remove LustreFileSystems finalizers")
	}

	if ready, err := r.isDaemonSetReady(ctx, manager); err != nil {
		return errorHandler(err, "check if daemonset is ready")
	} else if !ready {
		manager.Status.Ready = false
		log.Info("Daemonset not ready")
		return ctrl.Result{}, nil
	}

	manager.Status.Ready = true
	return ctrl.Result{}, nil
}

func (r *NnfDataMovementManagerReconciler) createSecretIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) (err error) {
	log := log.FromContext(ctx)

	newSecret := func() (*corev1.Secret, error) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating private key failed: %w", err)
		}
		privateDER, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("converting private key to DER format failed: %w", err)
		}

		privatePEM := pem.EncodeToMemory(&pem.Block{
			Type:  keyutil.ECPrivateKeyBlockType,
			Bytes: privateDER,
		})

		publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("generating public key failed: %w", err)
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      manager.Name,
				Namespace: manager.Namespace,
			},
			Type: corev1.SecretTypeSSHAuth,
			Data: map[string][]byte{
				corev1.SSHAuthPrivateKey: privatePEM,
				sshPublicKey:             ssh.MarshalAuthorizedKey(publicKey),
			},
		}

		if err := ctrl.SetControllerReference(manager, secret, r.Scheme); err != nil {
			return nil, fmt.Errorf("setting Secret controller reference failed: %w", err)
		}

		return secret, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name,
			Namespace: manager.Namespace,
		},
	}

	if err = r.Get(ctx, client.ObjectKeyFromObject(secret), secret); errors.IsNotFound(err) {
		secret, err = newSecret()
		if err != nil {
			return err
		}

		if err = r.Create(ctx, secret); err != nil {
			return err
		}

		log.Info("Created Secret", "object", client.ObjectKeyFromObject(secret).String())
	}

	return err
}

func (r *NnfDataMovementManagerReconciler) createOrUpdateDeploymentIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) (err error) {
	log := log.FromContext(ctx)

	base := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(manager), base); err != nil {
		return fmt.Errorf("retrieving base deployment failed %w", err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: manager.Namespace,
		},
	}

	mutateFn := func() error {
		deployment.Labels = base.Labels
		deployment.Spec = *base.Spec.DeepCopy()
		podSpec := &deployment.Spec.Template.Spec

		container, err := findContainer(podSpec, "manager")
		if err != nil {
			return err
		}

		for idx, arg := range container.Args {
			if arg == "--controller=manager" {
				container.Args[idx] = "--controller=default"
				break
			}
		}

		container.Args = append(container.Args, "--leader-elect=false") // TODO: Need to figure out how to make the deployment work with leader-election

		// Allow drive to reach workers through the service
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "OMPI_MCA_orte_keep_fqdn_hostnames",
			Value: "true",
		})

		setupSSHAuthVolumes(manager, podSpec)

		if err := ctrl.SetControllerReference(manager, deployment, r.Scheme); err != nil {
			return fmt.Errorf("setting Deployment controller reference failed: %w", err)
		}

		return nil
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, deployment, mutateFn)
	if err != nil {
		return err
	}

	if result == controllerutil.OperationResultCreated {
		log.Info("Created Deployment", "object", client.ObjectKeyFromObject(deployment).String())
	} else if result == controllerutil.OperationResultUpdated {
		log.Info("Updated Deployment", "object", client.ObjectKeyFromObject(deployment).String())
	}

	return nil
}

func (r *NnfDataMovementManagerReconciler) createOrUpdateServiceIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) error {
	log := log.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: manager.Namespace,
		},
	}

	mutateFn := func() error {
		service.Spec.Selector = map[string]string{
			nnfv1alpha7.DataMovementWorkerLabel: "true",
		}

		service.Spec.ClusterIP = corev1.ClusterIPNone

		if err := ctrl.SetControllerReference(manager, service, r.Scheme); err != nil {
			return fmt.Errorf("setting Service controller reference failed: %w", err)
		}

		return nil
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, service, mutateFn)
	if err != nil {
		return err
	}

	if result == controllerutil.OperationResultCreated {
		log.Info("Created Service", "object", client.ObjectKeyFromObject(service).String())
	} else if result == controllerutil.OperationResultUpdated {
		log.Info("Updated Service", "object", client.ObjectKeyFromObject(service).String())
	}

	return nil
}

func (r *NnfDataMovementManagerReconciler) updateLustreFileSystemsIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) error {
	log := log.FromContext(ctx)

	filesystems := &lusv1beta1.LustreFileSystemList{}
	if err := r.List(ctx, filesystems); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("list lustre file systems failed: %w", err)
	}

	for _, lustre := range filesystems.Items {
		_, found := lustre.Spec.Namespaces[manager.Namespace]
		if !found {
			if lustre.Spec.Namespaces == nil {
				lustre.Spec.Namespaces = make(map[string]lusv1beta1.LustreFileSystemNamespaceSpec)
			}

			lustre.Spec.Namespaces[manager.Namespace] = lusv1beta1.LustreFileSystemNamespaceSpec{
				Modes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			}
		}

		// Add the dm finalizer to keep this resource from being deleted until dm is no longer using it
		if lustre.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(&lustre, finalizer) {
			controllerutil.AddFinalizer(&lustre, finalizer)
		}

		if err := r.Update(ctx, &lustre); err != nil {
			return err
		}
		log.Info("Updated LustreFileSystem", "object", client.ObjectKeyFromObject(&lustre).String(), "namespace", manager.Namespace)
	}

	return nil
}

func (r *NnfDataMovementManagerReconciler) removeLustreFileSystemsFinalizersIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) error {
	log := log.FromContext(ctx)

	filesystems := &lusv1beta1.LustreFileSystemList{}
	if err := r.List(ctx, filesystems); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("list lustre file systems failed: %w", err)
	}

	// Get the DS to compare the list of volumes
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonsetName,
			Namespace: manager.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	finalizersToRemove := []lusv1beta1.LustreFileSystem{}
	for _, lustre := range filesystems.Items {
		// This lustre is in the process of deleting, verify it is not in the list of DS volumes
		if !lustre.DeletionTimestamp.IsZero() {
			finalizersToRemove = append(finalizersToRemove, lustre)
			for _, vol := range ds.Spec.Template.Spec.Volumes {
				if lustre.Name == vol.Name {
					log.Info("Daemonset still has lustrefilesystem volume", "lustrefilesystem", lustre)
					return nil
				}
			}
		}
	}

	if len(finalizersToRemove) == 0 {
		return nil
	}

	// Now the DS does not have any lustre filesystems that are being deleted, verify that the
	// daemonset's pods (i.e. dm worker pods) have restarted
	if ready, err := r.isDaemonSetReady(ctx, manager); !ready {
		log.Info("Daemonset still has pods to restart after dropping lustrefilesystem volume")
		return nil
	} else if err != nil {
		return err
	}

	// Now the finalizers can be removed
	for _, lustre := range finalizersToRemove {
		if controllerutil.ContainsFinalizer(&lustre, finalizer) {
			controllerutil.RemoveFinalizer(&lustre, finalizer)
			if err := r.Update(ctx, &lustre); err != nil {
				return err
			}
			log.Info("Removed LustreFileSystem finalizer", "object", client.ObjectKeyFromObject(&lustre).String(), "namespace", manager.Namespace)
		}
	}

	return nil
}

func (r *NnfDataMovementManagerReconciler) createOrUpdateDaemonSetIfNecessary(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) error {
	log := log.FromContext(ctx)

	filesystems := &lusv1beta1.LustreFileSystemList{}
	if err := r.List(ctx, filesystems); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("list lustre file systems failed: %w", err)
	}

	log.Info("LustreFileSystems", "count", len(filesystems.Items))

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonsetName,
			Namespace: manager.Namespace,
		},
	}

	mutateFn := func() error {
		podTemplateSpec := corev1.PodTemplateSpec{}

		// Add labels
		podTemplateSpec.Labels = manager.Spec.Selector.DeepCopy().MatchLabels
		if podTemplateSpec.Labels == nil {
			podTemplateSpec.Labels = make(map[string]string)
		}
		podTemplateSpec.Labels[nnfv1alpha7.DataMovementWorkerLabel] = "true"

		// Create corev1.PodSpec from NnfPodSpec
		podSpec := manager.Spec.PodSpec.ToCorePodSpec()
		podSpec.NodeSelector = manager.Spec.Selector.MatchLabels
		podSpec.Subdomain = serviceName
		podSpec.ServiceAccountName = "nnf-dm-node-controller"
		podSpec.Tolerations = []corev1.Toleration{
			{Key: "cray.nnf.node", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule},
		}
		podSpec.ShareProcessNamespace = pointy.Bool(true)

		if managerContainer, err := findContainer(podSpec, "manager"); err == nil {
			managerContainer.Env = append(managerContainer.Env, corev1.EnvVar{Name: "ENVIRONMENT", Value: os.Getenv("ENVIRONMENT")})
		}

		if workerContainer, err := findContainer(podSpec, "worker"); err == nil {
			workerContainer.Env = append(workerContainer.Env, corev1.EnvVar{Name: "ENVIRONMENT", Value: os.Getenv("ENVIRONMENT")})

			// Limit what the worker can do - this is enough to support dcp and being able to become
			// the the UID/GID of the workflow
			workerContainer.SecurityContext = &corev1.SecurityContext{
				Privileged: pointy.Bool(true),
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"SETUID",
						"SETGID",
						"MKNOD",
					},
				},
			}
		}

		setupSSHAuthVolumes(manager, podSpec)
		setupLustreVolumes(ctx, manager, podSpec, filesystems.Items)

		// Create the daemonset from the template spec
		podTemplateSpec.Spec = *podSpec
		updateStrategy := manager.Spec.UpdateStrategy.DeepCopy()
		ds.Spec = appsv1.DaemonSetSpec{
			Selector:       &manager.Spec.Selector,
			Template:       podTemplateSpec,
			UpdateStrategy: *updateStrategy,
		}

		if err := ctrl.SetControllerReference(manager, ds, r.Scheme); err != nil {
			return fmt.Errorf("setting DaemonSet controller reference failed: %w", err)
		}

		return nil
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, ds, mutateFn)
	if err != nil {
		return err
	}

	if result == controllerutil.OperationResultCreated {
		log.Info("Created DaemonSet", "object", client.ObjectKeyFromObject(ds).String())
	} else if result == controllerutil.OperationResultUpdated {
		log.Info("Updated DaemonSet", "object", client.ObjectKeyFromObject(ds).String(), "generation", ds.ObjectMeta.Generation, "status", ds.Status)
	}

	return nil
}

func (r *NnfDataMovementManagerReconciler) isDaemonSetReady(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager) (bool, error) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonsetName,
			Namespace: manager.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil

		}
		return false, err
	}

	// DS is not ready when the generations do not match, desired is 0 (e.g. no nnf nodes available), scheduled != desired, ready != desired
	d := ds.Status.DesiredNumberScheduled
	if ds.Status.ObservedGeneration != ds.ObjectMeta.Generation || d < 1 || ds.Status.UpdatedNumberScheduled != d || ds.Status.NumberReady != d {
		return false, nil
	}

	return true, nil
}

func setupSSHAuthVolumes(manager *nnfv1alpha7.NnfDataMovementManager, podSpec *corev1.PodSpec) {
	mode := int32(0600)
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: sshAuthVolume,

		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &mode,
				SecretName:  manager.Name,
				Items:       sshAuthVolumeItems,
			},
		},
	})

	for idx := range podSpec.Containers {
		container := &podSpec.Containers[idx]
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      sshAuthVolume,
			MountPath: "/root/.ssh",
		})
	}
}

func setupLustreVolumes(ctx context.Context, manager *nnfv1alpha7.NnfDataMovementManager, podSpec *corev1.PodSpec, fileSystems []lusv1beta1.LustreFileSystem) {
	log := log.FromContext(ctx)

	// Setup Volumes / Volume Mounts for accessing global Lustre file systems

	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	for _, fs := range fileSystems {

		if !fs.DeletionTimestamp.IsZero() {
			log.Info("Global lustre volume is in the process of being deleted", "name", client.ObjectKeyFromObject(&fs).String())
			continue
		}

		log.Info("Adding global lustre volume", "name", client.ObjectKeyFromObject(&fs).String())

		volumes = append(volumes, corev1.Volume{
			Name: fs.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: fs.PersistentVolumeClaimName(manager.Namespace, corev1.ReadWriteMany),
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      fs.Name,
			MountPath: fs.Spec.MountRoot,
		})
	}

	// Add the NNF Mounts
	hostPathType := corev1.HostPathDirectoryOrCreate
	volumes = append(volumes, corev1.Volume{
		Name: nnfVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: manager.Spec.HostPath,
				Type: &hostPathType,
			},
		},
	})

	podSpec.Volumes = append(podSpec.Volumes, volumes...)

	// Setup the mounts for each container in the pod spec.
	mountPropagation := corev1.MountPropagationHostToContainer
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:             nnfVolumeName,
		MountPath:        manager.Spec.MountPath,
		MountPropagation: &mountPropagation,
	})

	for idx := range podSpec.Containers {
		container := &podSpec.Containers[idx]
		container.VolumeMounts = append(container.VolumeMounts, volumeMounts...)
	}
}

func findContainer(podSpec *corev1.PodSpec, name string) (*corev1.Container, error) {
	name = strings.ToLower(name)
	for idx, container := range podSpec.Containers {
		if container.Name == name {
			return &podSpec.Containers[idx], nil
		}
	}

	return nil, fmt.Errorf("could not locate '%s' container in pod spec", name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NnfDataMovementManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha7.NnfDataMovementManager{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.DaemonSet{}).
		Watches(
			&lusv1beta1.LustreFileSystem{},
			handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      "nnf-dm-manager-controller-manager",
							Namespace: nnfv1alpha7.DataMovementNamespace,
						},
					},
				}
			}),
		).
		Complete(r)
}
