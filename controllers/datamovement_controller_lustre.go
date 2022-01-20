package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubeflowv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"

	lustrecsi "github.hpe.com/hpe/hpc-rabsw-lustre-csi-driver/pkg/lustre-driver/service"
	lustrectrl "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/controllers"
)

const (
	persistentVolumeSuffix      = "-pv"
	persistentVolumeClaimSuffix = "-pvc"
	mpiJobSuffix                = "-mpi"
	configSuffix                = "-mpi-config"
)

const (
	configImage             = "image"             // Image specifies the image used in the MPI launcher & worker containers
	configCommand           = "command"           // Command specifies the command to run. Defaults to "mpirun"
	configSourceVolume      = "sourceVolume"      // SourceVolume is the corev1.VolumeSource used as the source volume mount. Defaults to a CSI volume interpreted from the Spec.Source
	configDestinationVolume = "destinationVolume" // DestinationVolume is the corev1.VolumeSource used as the destination volume mount. Defaults to a CSI volume interpreted from the Spec.Destination
)

func (r *DataMovementReconciler) initializeLustreJob(ctx context.Context, dm *dmv1alpha1.DataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx, "DataMovement", "Lustre")

	// We need to label all the nodes in the Servers object with a unique label that describes this
	// data movememnt. This label is then used as a selector within the the MPIJob so it correctly
	// targets all the nodes
	result, err := r.labelStorageNodes(ctx, dm)
	if err != nil {
		log.Error(err, "Failed to label storage nodes")
		return nil, err
	} else if !result.IsZero() {
		return result, nil
	}

	// TODO: The PV/PVC should only be needed for JobStorageInstance, otherwise the PV/PVC will already
	//       be created, and we only need to reference them in the MPIJob.

	if err := r.createPersistentVolume(ctx, dm); err != nil {
		log.Error(err, "Failed to create persistent volume")
		return nil, err
	}

	if err := r.createPersistentVolumeClaim(ctx, dm); err != nil {
		log.Error(err, "Failed to create persistent volume claim")
		return nil, err
	}

	if err := r.createMpiJob(ctx, dm); err != nil {
		log.Error(err, "Failed to create MPI Job")
		return nil, err
	}

	log.Info("Data Movement Started")
	return nil, nil
}

func (r *DataMovementReconciler) labelStorageNodes(ctx context.Context, dm *dmv1alpha1.DataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("labeler")

	// List of target node names that are to perform lustre data movement
	targetNodeNames := make([]string, 0)

	// DEVELOPMENT: Support the NnfStorage as an allocation mechanism; this should be replaced by DWS Servers definition,
	// but for development the NnfStorage is easier to use for both listing the nodes and getting the msgNode.
	switch dm.Spec.Storage.Kind {
	case "NnfStorage":
		storageRef := dm.Spec.Storage
		storage := &nnfv1alpha1.NnfStorage{}
		if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
			return nil, err
		}

		targetAllocationSetIndex := -1
		for allocationSetIndex, allocationSet := range storage.Spec.AllocationSets {
			if allocationSet.TargetType == "OST" {
				targetAllocationSetIndex = allocationSetIndex
			}
		}

		if targetAllocationSetIndex == -1 {
			return nil, fmt.Errorf("OST allocation set not found")
		}

		for _, storageNode := range storage.Spec.AllocationSets[targetAllocationSetIndex].Nodes {
			targetNodeNames = append(targetNodeNames, storageNode.Name)
		}
		break
	}

	// Retrieve all the NNF Nodes in the cluster - these nodes will be matched against the requested
	// node list and labeled such that the mpijob can target the desired nodes
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.HasLabels{"cray.nnf.node"}); err != nil {
		return nil, err
	}

	label := dm.Name

	for _, nodeName := range targetNodeNames {

		nodeFound := false
		for _, node := range nodes.Items {
			if node.Name == nodeName {
				if _, found := node.Labels[label]; !found {

					node.Labels[label] = "true"

					log.Info("Applying label to node", "node", node.Name)
					if err := r.Update(ctx, &node); err != nil {
						if errors.IsConflict(err) {
							return &ctrl.Result{Requeue: true}, nil
						}

						return nil, err
					}
				}

				nodeFound = true
				break
			}
		}

		if !nodeFound {
			log.Info("Node %s not found. Check the spelling or status of the node")
		}
	}

	return nil, nil
}

func (r *DataMovementReconciler) unlabelStorageNodes(ctx context.Context, dm *dmv1alpha1.DataMovement) (*ctrl.Result, error) {

	label := dm.Name
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.HasLabels{label}); err != nil {
		return nil, err
	}

	for _, node := range nodes.Items {
		delete(node.Labels, label)
		if err := r.Update(ctx, &node); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (r *DataMovementReconciler) createPersistentVolume(ctx context.Context, dm *dmv1alpha1.DataMovement) error {

	if dm.Spec.Storage.Kind != "NnfStorage" {
		panic("Create Persistent Volume requires NNF Storage type")
	}

	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Storage.Name, Namespace: dm.Spec.Storage.Namespace}, storage); err != nil {
		return err
	}

	volumeMode := corev1.PersistentVolumeFilesystem
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + persistentVolumeSuffix,
			Namespace: dm.Namespace,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1"),
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       lustrecsi.Name,
					FSType:       "lustre",
					VolumeHandle: storage.Status.MgsNode,
				},
			},
			VolumeMode: &volumeMode,
		},
	}

	ctrl.SetControllerReference(pv, dm, r.Scheme)

	if err := r.Create(ctx, pv); err != nil {
		if !errors.IsAlreadyExists(err) { // Ignore existing which may occur while trying to create the resource subtree fails at a later step
			return err
		}
	}

	return nil
}

func (r *DataMovementReconciler) createPersistentVolumeClaim(ctx context.Context, dm *dmv1alpha1.DataMovement) error {

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + persistentVolumeClaimSuffix,
			Namespace: dm.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: dm.Name + persistentVolumeSuffix,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1"),
				},
			},
		},
	}

	ctrl.SetControllerReference(pvc, dm, r.Scheme)

	if err := r.Create(ctx, pvc); err != nil {
		if !errors.IsAlreadyExists(err) { // Ignore existing which may occur while trying to create the resource subtree fails at a later step
			return err
		}
	}

	return nil
}

func (r *DataMovementReconciler) createMpiJob(ctx context.Context, dm *dmv1alpha1.DataMovement) error {

	config, err := r.getDataMovementConfigMap(ctx)
	if err != nil {
		return err
	}

	image := "arti.dev.cray.com/rabsw-docker-master-local/mfu:0.0.1"
	if img, found := config.Data[configImage]; found {
		image = img
	}

	command := []string{"mpirun", "--allow-run-as-root", "dcp", "/mnt/src" + dm.Spec.Source.Path, "/mnt/dest" + dm.Spec.Destination.Path}
	if cmd, found := config.Data[configCommand]; found {
		if strings.HasPrefix(cmd, "/bin/bash -c") {
			command = []string{"/bin/bash", "-c", strings.TrimPrefix(cmd, "/bin/bash -c")}
		} else {
			command = strings.Split(cmd, " ")
		}

	}

	launcher := &kubeflowv1.ReplicaSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image:   image,
						Name:    dm.Name,
						Command: command,
					},
				},
			},
		},
	}

	replicas := int32(1) // TODO: This should be number of nodes
	worker := &kubeflowv1.ReplicaSpec{
		Replicas: &replicas,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					dm.Name: "true",
				},
			},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					dm.Name: "true",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "cray.nnf.node",
						Operator: corev1.TolerationOpEqual,
						Value:    "true",
						Effect:   "NoSchedule",
					},
				},
				Affinity: &corev1.Affinity{
					// Prevent multiple mpi-workers from being scheduled on the same node. That is to say...
					// The pod should _not_ be scheduled (through anti-affinity) onto a node if that node
					// is in the same zone (dm.Name) as a pod having label dm.Name="true".
					PodAntiAffinity: &corev1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
							{
								Weight: 100,
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											dm.Name: "true",
										},
									},
									TopologyKey: dm.Name,
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Image: config.Data[configImage],
						Name:  dm.Name,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "source",
								MountPath: "/mnt/src",
							},
							{
								Name:      "destination",
								MountPath: "/mnt/dest",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name:         "source",
						VolumeSource: r.getVolumeSource(ctx, dm, config, configSourceVolume, r.getLustreSourcePersistentVolumeClaimName),
					},
					{
						Name:         "destination",
						VolumeSource: r.getVolumeSource(ctx, dm, config, configDestinationVolume, r.getLustreDestinationPersistentVolumeClaimName),
					},
				},
			},
		},
	}

	job := &mpiv2beta1.MPIJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + mpiJobSuffix,
			Namespace: dm.Namespace,
		},
		Spec: mpiv2beta1.MPIJobSpec{
			MPIReplicaSpecs: map[mpiv2beta1.MPIReplicaType]*kubeflowv1.ReplicaSpec{
				mpiv2beta1.MPIReplicaTypeLauncher: launcher,
				mpiv2beta1.MPIReplicaTypeWorker:   worker,
			},
		},
	}

	ctrl.SetControllerReference(job, dm, r.Scheme)

	return r.Create(ctx, job)
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

func (r *DataMovementReconciler) getVolumeSource(ctx context.Context, dm *dmv1alpha1.DataMovement, config *corev1.ConfigMap, override string, claimFn func(context.Context, *dmv1alpha1.DataMovement) string) corev1.VolumeSource {
	if data, found := config.Data[override]; found {
		source := corev1.VolumeSource{}
		if err := json.Unmarshal([]byte(data), &source); err == nil {
			return source
		} else {
			log.FromContext(ctx).Info("Failed to unmarshal override config " + override)
		}
	}

	return corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: claimFn(ctx, dm),
		},
	}
}

func (r *DataMovementReconciler) getLustreSourcePersistentVolumeClaimName(ctx context.Context, dm *dmv1alpha1.DataMovement) string {
	ref := dm.Spec.Source.StorageInstance

	if ref.Kind == "LustreFileSystem" {
		return ref.Name + lustrectrl.PersistentVolumeClaimSuffix
	} else if ref.Kind == "NnfPersistentStorageInstance" {
		return ref.Name + "TODO" // TODO: Should look this up via nnf-sos when creating persistent lustre volume
	} else if ref.Kind == "NnfJobStorageInstance" {
		return dm.Name + persistentVolumeClaimSuffix
	}

	panic("Unsupported Lustre Source PVC: " + ref.Kind)
}

func (r *DataMovementReconciler) getLustreDestinationPersistentVolumeClaimName(ctx context.Context, dm *dmv1alpha1.DataMovement) string {
	ref := dm.Spec.Destination.StorageInstance

	if ref.Kind == "LustreFileSystem" {
		return ref.Name + lustrectrl.PersistentVolumeClaimSuffix
	} else if ref.Kind == "NnfPersistentStorageInstance" {
		return ref.Name + "TODO" // TODO: Should look this up via nnf-sos when creating persistent lustre volume
	} else if ref.Kind == "NnfJobStorageInstance" {
		return dm.Name + persistentVolumeClaimSuffix
	}

	panic("Unsupported Lustre Destination PVC: " + ref.Kind)
}

func (r *DataMovementReconciler) monitorLustreJob(ctx context.Context, dm *dmv1alpha1.DataMovement) (*ctrl.Result, error) {

	job := &mpiv2beta1.MPIJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + mpiJobSuffix,
			Namespace: dm.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		return nil, err
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == kubeflowv1.JobFailed {
			return nil, nil // TODO: Propagate error up to caller
		} else if condition.Type == kubeflowv1.JobSucceeded {
			return nil, nil
		}

	}

	// TODO: Consider the job still running, we'll pick up the next event through the watch

	return &ctrl.Result{}, nil
}
