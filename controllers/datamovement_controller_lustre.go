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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubeflowv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"

	lustrecsi "github.com/HewlettPackard/lustre-csi-driver/pkg/lustre-driver/service"
	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	lustrectrl "github.com/NearNodeFlash/lustre-fs-operator/controllers"
)

const (
	MPIJobOwnerNameLabel = "dm.cray.hpe.com/owner.name"

	MPIJobOwnerNamespaceLabel = "dm.cray.hpe.com/owner.namespace"
)

const (
	persistentVolumeSuffix      = "-pv"
	persistentVolumeClaimSuffix = "-pvc"
	mpiJobSuffix                = "-mpi"
	configSuffix                = "-config"
	configNamespace             = "nnf-dm-system"
)

const (
	configImage             = "image"             // Image specifies the image used in the MPI launcher & worker containers
	configCommand           = "command"           // Command specifies the command to run. Defaults to "mpirun" or "rsync"
	configSourcePath        = "sourcePath"        // SourcePath is the path of the source file or directory
	configDestinationPath   = "destinationPath"   // DestinationPath is the path of the destination file or directory
	configSourceVolume      = "sourceVolume"      // SourceVolume is the corev1.VolumeSource used as the source volume mount. Defaults to a CSI volume interpreted from the Spec.Source
	configDestinationVolume = "destinationVolume" // DestinationVolume is the corev1.VolumeSource used as the destination volume mount. Defaults to a CSI volume interpreted from the Spec.Destination

	mpiArguments = "mpi-arguments" // Additional arguments to pass to the mpirun command
)

func (r *DataMovementReconciler) initializeLustreJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx, "DataMovement", "Lustre")

	// We need to label all the nodes in the NNF Storage object with a unique label that describes this
	// data movememnt. This label is then used as a selector within the the MPIJob so it correctly
	// targets all the nodes
	result, workerCount, err := r.labelStorageNodes(ctx, dm)
	if err != nil {
		log.Error(err, "Failed to label storage nodes")
		return nil, err
	} else if !result.IsZero() {
		return result, nil
	}

	// TODO: The PV/PVC should only be needed for NnfStorage, otherwise the PV/PVC will already
	//       be created, and we only need to reference them in the MPIJob.

	if err := r.createPersistentVolume(ctx, dm); err != nil {
		log.Error(err, "Failed to create persistent volume")
		return nil, err
	}

	if err := r.createPersistentVolumeClaim(ctx, dm); err != nil {
		log.Error(err, "Failed to create persistent volume claim")
		return nil, err
	}

	if err := r.createMpiJob(ctx, dm, workerCount); err != nil {
		log.Error(err, "Failed to create MPI Job")
		return nil, err
	}

	log.Info("Lustre data movement initialized")
	return nil, nil
}

func (r *DataMovementReconciler) labelStorageNodes(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, int32, error) {
	log := log.FromContext(ctx).WithName("label")

	var storageRef *corev1.ObjectReference
	if dm.Spec.Source.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Source.Storage
	} else if dm.Spec.Destination.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Destination.Storage
	} else {
		return nil, -1, fmt.Errorf("Neither source or destination is of NNF Storage type")
	}

	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
		return nil, -1, err
	}

	targetAllocationSetIndex := -1
	for allocationSetIndex, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.TargetType == "OST" {
			targetAllocationSetIndex = allocationSetIndex
		}
	}

	if targetAllocationSetIndex == -1 {
		return nil, -1, fmt.Errorf("OST allocation set not found")
	}

	targetNodeNames := make([]string, 0) // List of target node names that are to perform lustre data movement
	for _, storageNode := range storage.Spec.AllocationSets[targetAllocationSetIndex].Nodes {
		targetNodeNames = append(targetNodeNames, storageNode.Name)
	}

	// Retrieve all the NNF Nodes in the cluster - these nodes will be matched against the requested
	// node list and labeled such that the mpijob can target the desired nodes
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.HasLabels{"cray.nnf.node"}); err != nil {
		return nil, -1, err
	}

	label := dm.Name
	log.Info("Labeling nodes", "label", label, "count", len(targetNodeNames))

	for _, nodeName := range targetNodeNames {

		nodeFound := false
		for _, node := range nodes.Items {

			if node.Name == nodeName {
				if _, found := node.Labels[label]; !found {

					node.Labels[label] = "true"

					log.Info("Applying label to node", "node", nodeName)
					if err := r.Update(ctx, &node); err != nil {
						if errors.IsConflict(err) {
							return &ctrl.Result{Requeue: true}, -1, nil
						}

						return nil, -1, err
					}
				}

				nodeFound = true
				break
			}
		}

		if !nodeFound {
			log.Info("Node not found. Check the spelling or status of the node.", "node", nodeName)
		}
	}

	return nil, int32(len(targetNodeNames)), nil
}

func (r *DataMovementReconciler) teardownLustreJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("unlabel")

	label := dm.Name
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.HasLabels{label}); err != nil {
		return nil, err
	}

	log.Info("Unlabeling nodes", "count", len(nodes.Items))
	for _, node := range nodes.Items {
		delete(node.Labels, label)
		if err := r.Update(ctx, &node); err != nil {
			return nil, err
		}
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: dm.Name + persistentVolumeSuffix,
		},
	}

	if err := r.Delete(ctx, pv); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + persistentVolumeClaimSuffix,
			Namespace: "nnf-dm-system",
		},
	}

	if err := r.Delete(ctx, pvc); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	job := &mpiv2beta1.MPIJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + mpiJobSuffix,
			Namespace: "nnf-dm-system",
		},
	}

	if err := r.Delete(ctx, job); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (r *DataMovementReconciler) createPersistentVolume(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) error {
	log := log.FromContext(ctx).WithName("pv")

	var storageRef *corev1.ObjectReference
	if dm.Spec.Source.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Source.Storage
	} else if dm.Spec.Destination.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Destination.Storage
	} else {
		return fmt.Errorf("Neither source or destination is of NNF Storage type")
	}

	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
		return err
	}

	fsName := ""
	for _, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.TargetType == "MDT" || allocationSet.TargetType == "MGTMDT" {
			fsName = allocationSet.FileSystemName
		}
	}

	if len(fsName) == 0 {
		return fmt.Errorf("File System Name not found in NNF Storage spec")
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: dm.Name + persistentVolumeSuffix,
		},
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, pv, func() error {

		volumeMode := corev1.PersistentVolumeFilesystem
		pv.Spec = corev1.PersistentVolumeSpec{
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
					VolumeHandle: storage.Status.MgsNode + ":/" + fsName,
				},
			},
			VolumeMode:       &volumeMode,
			StorageClassName: "nnf-lustre-fs",
		}

		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create persistent volume")
		return err
	} else if result == controllerutil.OperationResultCreated {
		log.V(2).Info("Created persistent volume", "object", client.ObjectKeyFromObject(pv).String())
	}

	return nil
}

func (r *DataMovementReconciler) createPersistentVolumeClaim(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) error {
	log := log.FromContext(ctx).WithName("pvc")

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + persistentVolumeClaimSuffix,
			Namespace: "nnf-dm-system",
		},
	}

	storageClassName := "nnf-lustre-fs"
	result, err := ctrl.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			VolumeName: dm.Name + persistentVolumeSuffix,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1"),
				},
			},
			StorageClassName: &storageClassName,
		}

		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create persistent volume claim")
		return err
	} else if result == controllerutil.OperationResultCreated {
		log.V(2).Info("Created persistent volume claim", "object", client.ObjectKeyFromObject(pvc).String())
	}

	return nil
}

func (r *DataMovementReconciler) createMpiJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement, workerCount int32) error {
	log := log.FromContext(ctx)

	config, err := r.getDataMovementConfigMap(ctx)
	if err != nil {
		log.Error(err, "Failed to read mpi configuration")
		return err
	}

	image := "ghcr.io/nearnodeflash/nnf-mfu:latest"
	if img, found := config.Data[configImage]; found {
		image = img
	}

	sourceMount := "/mnt/src"
	sourcePath := sourceMount + dm.Spec.Source.Path
	if dm.Spec.Source.Storage.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {

		lustre := &lusv1alpha1.LustreFileSystem{}
		if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Source.Storage.Name, Namespace: dm.Spec.Source.Storage.Namespace}, lustre); err != nil {
			return err
		}

		sourceMount = lustre.Spec.MountRoot
		sourcePath = dm.Spec.Source.Path
	}

	destinationMount := "/mnt/dest"
	destinationPath := destinationMount + dm.Spec.Destination.Path
	if dm.Spec.Destination.Storage.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {

		lustre := &lusv1alpha1.LustreFileSystem{}
		if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Destination.Storage.Name, Namespace: dm.Spec.Destination.Storage.Namespace}, lustre); err != nil {
			return err
		}

		destinationMount = lustre.Spec.MountRoot
		destinationPath = dm.Spec.Destination.Path
	}

	command := []string{"mpirun", "--allow-run-as-root"}
	if arguments, found := config.Data[mpiArguments]; found {
		command = append(command, strings.Split(arguments, " ")...)
	}

	command = append(command, "dcp", sourcePath, destinationPath)

	if cmd, found := config.Data[configCommand]; found {
		if strings.HasPrefix(cmd, "/bin/bash -c") {
			command = []string{"/bin/bash", "-c", strings.TrimPrefix(cmd, "/bin/bash -c")}
		} else {
			command = strings.Split(cmd, " ")
		}
		log.V(1).Info("Command override", "command", command)
	}

	userId := int64(dm.Spec.UserId)
	groupId := int64(dm.Spec.GroupId)

	launcher := &kubeflowv1.ReplicaSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image:   image,
						Name:    dm.Name,
						Command: command,
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:  &userId,
							RunAsGroup: &groupId,
						},
					},
				},
			},
		},
	}

	replicas := workerCount
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
						Image: image,
						Name:  dm.Name,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "source",
								MountPath: sourceMount,
							},
							{
								Name:      "destination",
								MountPath: destinationMount,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:  &userId,
							RunAsGroup: &groupId,
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

	sshAuthMountPath := "" // Implies default /root/.ssh

	if userId != 0 || groupId != 0 {
		// This is stolen from https://github.com/kubeflow/mpi-operator/blob/master/examples/v2beta1/pi/pi.yaml which
		// contains an example of using non-root user
		worker.Template.Spec.Containers[0].Command = []string{"/usr/sbin/sshd"}
		worker.Template.Spec.Containers[0].Args = []string{"-De", "-f", "/home/mpiuser/.sshd_config"}

		sshAuthMountPath = "/home/mpiuser/.ssh"
	}

	job := &mpiv2beta1.MPIJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + mpiJobSuffix,
			Namespace: "nnf-dm-system",
			Labels: map[string]string{
				MPIJobOwnerNameLabel:      dm.Name,
				MPIJobOwnerNamespaceLabel: dm.Namespace,
			},
		},
		Spec: mpiv2beta1.MPIJobSpec{
			MPIReplicaSpecs: map[mpiv2beta1.MPIReplicaType]*kubeflowv1.ReplicaSpec{
				mpiv2beta1.MPIReplicaTypeLauncher: launcher,
				mpiv2beta1.MPIReplicaTypeWorker:   worker,
			},
			SSHAuthMountPath: sshAuthMountPath,
		},
	}

	//ctrl.SetControllerReference(dm, job, r.Scheme)

	log.Info("Creating mpi job", "name", client.ObjectKeyFromObject(job).String())
	return r.Create(ctx, job)
}

func (r *DataMovementReconciler) getVolumeSource(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement, config *corev1.ConfigMap, override string, claimFn func(context.Context, *nnfv1alpha1.NnfDataMovement) string) corev1.VolumeSource {
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

func (r *DataMovementReconciler) getLustreSourcePersistentVolumeClaimName(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) string {
	ref := dm.Spec.Source.Storage

	if ref.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {
		return ref.Name + lustrectrl.PersistentVolumeClaimSuffix
	} else if ref.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		return dm.Name + persistentVolumeClaimSuffix
	}

	panic("Unsupported Lustre Source PVC: " + ref.Kind)
}

func (r *DataMovementReconciler) getLustreDestinationPersistentVolumeClaimName(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) string {
	ref := dm.Spec.Destination.Storage

	if ref.Kind == reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name() {
		return ref.Name + lustrectrl.PersistentVolumeClaimSuffix
	} else if ref.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		return dm.Name + persistentVolumeClaimSuffix
	}

	panic("Unsupported Lustre Destination PVC: " + ref.Kind)
}

func (r *DataMovementReconciler) monitorLustreJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, string, string, error) {

	job := &mpiv2beta1.MPIJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Name + mpiJobSuffix,
			Namespace: "nnf-dm-system",
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		return nil, nnfv1alpha1.DataMovementConditionReasonFailed, "ObjectNotFound", err
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == kubeflowv1.JobFailed {
			return nil, nnfv1alpha1.DataMovementConditionReasonFailed, condition.Message, nil
		} else if condition.Type == kubeflowv1.JobSucceeded {
			return nil, nnfv1alpha1.DataMovementConditionReasonSuccess, condition.Message, nil
		}
	}

	// Consider the job still running
	return &ctrl.Result{}, nnfv1alpha1.DataMovementConditionTypeRunning, "Running", nil
}

func mpijobEnqueueRequestMapFunc(o client.Object) []reconcile.Request {
	labels := o.GetLabels()

	ownerName, exists := labels[MPIJobOwnerNameLabel]
	if exists == false {
		return []reconcile.Request{}
	}

	ownerNamespace, exists := labels[MPIJobOwnerNamespaceLabel]
	if exists == false {
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      ownerName,
				Namespace: ownerNamespace,
			},
		},
	}
}
