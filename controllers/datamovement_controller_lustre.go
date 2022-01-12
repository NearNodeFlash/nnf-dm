package controllers

import (
	"context"

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
)

const (
	persistentVolumeSuffix      = "-pv"
	persistentVolumeClaimSuffix = "-pvc"
	mpiJobSuffix                = "-mpi"
)

func (r *DataMovementReconciler) initializeLustreJob(ctx context.Context, dm *dmv1alpha1.DataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx, "DataMovement", "Lustre")

	// LUSTRE 2 LUSTRE
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
	//
	// When Destination is

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
	storageRef := dm.Spec.Storage
	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
		return nil, err
	}

	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.HasLabels{"cray.nnf.node"}); err != nil {
		return nil, err
	}

	label := dm.Name

	for _, storageNode := range storage.Spec.AllocationSets[0].Nodes {
		for _, node := range nodes.Items {

			if node.Name == storageNode.Name {
				if _, found := node.Labels[label]; !found {

					node.Labels[label] = "true"

					if err := r.Update(ctx, &node); err != nil {
						if errors.IsConflict(err) {
							return &ctrl.Result{Requeue: true}, nil
						}

						return nil, err
					}
				}

				break
			}
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
					Driver:       "TODO",
					FSType:       "lustre",
					VolumeHandle: "TODO",
				},
			},
			VolumeMode: &volumeMode,
		},
	}

	ctrl.SetControllerReference(pv, dm, r.Scheme)

	return r.Create(ctx, pv)
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

	return r.Create(ctx, pvc)
}

func (r *DataMovementReconciler) createMpiJob(ctx context.Context, dm *dmv1alpha1.DataMovement) error {

	launcher := &kubeflowv1.ReplicaSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image:   "", // This should be from some config file somewhere
						Name:    dm.Name,
						Command: []string{},
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
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Image: "", // This should be from some config file somewhere,
						Name:  dm.Name,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "source",
								MountPath: dm.Spec.Source.Path,
							},
							{
								Name:      "destination",
								MountPath: dm.Spec.Destination.Path,
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "source",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								// TODO: If SOURCE is global lustre, we should Get() the claim name from the lustre-fs-operator
								//       If SOURCE is persistent lustre, we get the name from the PersistentVolumeSource
								ClaimName: "",
							},
						},
					},
					{
						Name: "destination",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								// TODO: If DESTINATION is persistent lustre, we get the name from the PersistentVolumeSource
								//       If DESTINATION is job lustre, we know the name is for our self created PVC
								ClaimName: "",
							},
						},
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
