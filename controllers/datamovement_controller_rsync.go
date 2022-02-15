package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lusv1alpha1 "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

const (
	rsyncSuffix = "-rsync"

	ownerLabelRsyncNodeDataMovement = "dm.cray.hpe.com/owner"
)

func (r *DataMovementReconciler) initializeRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {

	nodes, err := r.getStorageNodes(ctx, dm)
	if err != nil {
		return nil, err
	}

	return r.startNodeDataMovers(ctx, dm, nodes)
}

func (r *DataMovementReconciler) getStorageNodes(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) ([]nnfv1alpha1.NnfStorageAllocationNodes, error) {

	// Retrieve the NnfStorage object that is associated with this Rsync job. This provides the list of Rabbits that will receive
	// RsyncNodeDataMovement resources.
	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Storage.Name, Namespace: dm.Spec.Storage.Namespace}, storage); err != nil {
		return nil, err
	}

	// The NnfStorage specification is a list of Allocation Sets; with each set containing a number of Nodes <Name, Count> pair that
	// describes the Rabbit and the number of allocations to perform on that node. Since this is an Rsync job, the expected number
	// of Allocation Sets is one, and it should be of xfs/gfs2 type.
	for _, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.FileSystemType == "xfs" || allocationSet.FileSystemType == "gfs2" {
			return allocationSet.Nodes, nil
		}
	}

	return nil, fmt.Errorf("Invalid NnfStorage: Must have xfs/gfs2 allocation set")
}

func (r *DataMovementReconciler) startNodeDataMovers(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement, nodes []nnfv1alpha1.NnfStorageAllocationNodes) (*ctrl.Result, error) {
	log := log.FromContext(ctx)

	config, err := r.getDataMovementConfigMap(ctx)
	if err != nil {
		return nil, err
	}

	// From the provided NnfAccess, we'll need the source/destination to load the PrefixPath; this is the
	// path that is the basis for this data movement request, and we should append "/compute-%id" onto the prefix
	// path, and then append the rsync.Spec.Source/rsync.Spec.Destination as needed.
	//
	// For example, if a request is for XFS Storage, with Source=file.in Destination=file.out, NnfAccess will contain
	// a prefix path that corresponds to the XFS File System at something like /mnt/nnf/job-1234/. In this case
	// we would create rsync jobs with Destination=/mnt/nnf/job-1234/compute-%id/file.out

	access := &nnfv1alpha1.NnfAccess{
		ObjectMeta: metav1.ObjectMeta{
			Name: dm.Spec.Access.Name,
			Namespace: dm.Spec.Access.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(access), access); err != nil {
		return nil, err
	}

	for _, node := range nodes {
		log.V(1).Info("Creating Rsync Node Data Movement", "node", node.Name, "count", node.Count)

		dm.Status.NodeStatus = append(dm.Status.NodeStatus, nnfv1alpha1.NnfDataMovementNodeStatus{
			Node:     node.Name,
			Count:    uint32(node.Count),
			Running:  0,
			Complete: 0,
		})

		for i := 0; i < node.Count; i++ {
			rsyncNode := &dmv1alpha1.RsyncNodeDataMovement{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%d", dm.Name, i),
					Namespace: node.Name,
					Labels: map[string]string{
						// Bare name (without namespace) is used to List() all the Rsync Nodes by label. 
						// This requires dm.Name to be unique across Namespaces, which is always the case when
						// data movement resource is created from a workflow.
						// We can't append "/namespace" here because of the label regex validation rules do not
						// permit the forward slash "/"
						ownerLabelRsyncNodeDataMovement: dm.Name, 
					},
					Annotations: map[string]string{
						// Annotation is used to watch Rsync Nodes and reconcile the Data Movement resource
						// when they change. This is namespace scoped.
						ownerLabelRsyncNodeDataMovement: dm.Name + "/" + dm.Namespace,
					},
				},
				Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
					Source:      r.getRsyncPath(dm.Spec.Source, config, access.Spec.MountPathPrefix, i, configSourcePath),
					Destination: r.getRsyncPath(dm.Spec.Destination, config, access.Spec.MountPathPrefix, i, configDestinationPath),
				},
			}

			if err := r.Create(ctx, rsyncNode); err != nil {
				if !errors.IsAlreadyExists(err) {
					log.V(1).Error(err, "Failed to create rsync node")
					return nil, err
				}
			}
		}

		log.V(1).Info("Created Rsync Node Data Movement resources", "node", node.Name, "count", node.Count)
	}

	return nil, nil
}

func (r *DataMovementReconciler) getRsyncPath(spec nnfv1alpha1.NnfDataMovementSpecSourceDestination, config *corev1.ConfigMap, prefixPath string, index int, override string) string {
	if path, found := config.Data[override]; found {
		return path
	}

	if spec.StorageInstance == nil {
		return spec.Path
	}
	switch spec.StorageInstance.Kind {
	case reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name():
		return spec.Path
	case reflect.TypeOf(nnfv1alpha1.NnfJobStorageInstance{}).Name(), "NnfPersistentStorageInstance":
		return prefixPath + fmt.Sprintf("/compute-%d", index) + spec.Path
	}

	panic(fmt.Sprintf("Unsupported Storage Instance %s", spec.StorageInstance.Kind))
}

func (r *DataMovementReconciler) monitorRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, string, string, error) {
	log := log.FromContext(ctx)

	nodes := &dmv1alpha1.RsyncNodeDataMovementList{}
	if err := r.List(ctx, nodes, client.MatchingLabels{ownerLabelRsyncNodeDataMovement: dm.Name}); err != nil {
		return nil, "", "", err
	}

	// Create a map by node name so the status' can be refreshed
	statusMap := map[string]*nnfv1alpha1.NnfDataMovementNodeStatus{}
	for statusIdx, status := range dm.Status.NodeStatus {
		dm.Status.NodeStatus[statusIdx].Complete = 0
		dm.Status.NodeStatus[statusIdx].Running = 0
		statusMap[status.Node] = &dm.Status.NodeStatus[statusIdx]
	}

	for _, node := range nodes.Items {

		status, found := statusMap[node.Namespace]
		if !found {
			log.Info("Node not found", "node", node.Namespace)
			continue
		}

		log.Info("Refresh Node Status", "node", node.Namespace, "state", node.Status.State, "message", node.Status.Message)

		switch node.Status.State {
		case nnfv1alpha1.DataMovementConditionTypeRunning:
			status.Running++
		case nnfv1alpha1.DataMovementConditionTypeFinished:
			status.Complete++
		}

		if len(node.Status.Message) != 0 {
			status.Messages = append(status.Messages, node.Status.Message)
		}
	}

	currentStatus := nnfv1alpha1.DataMovementConditionReasonSuccess
	currentMessage := ""
	for _, status := range dm.Status.NodeStatus {
		if status.Complete < status.Count {
			currentStatus = nnfv1alpha1.DataMovementConditionTypeRunning
		}
		if len(status.Messages) != 0 {
			currentStatus = nnfv1alpha1.DataMovementConditionReasonFailed
			currentMessage = "Failure detected on nodes TODO" // TODO: Talk with Dean on how we want to report rsync errors
		}
	}

	return nil, currentStatus, currentMessage, nil

}

func (r *DataMovementReconciler) teardownRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx)

	rsyncNodes := &dmv1alpha1.RsyncNodeDataMovementList{}
	if err := r.List(ctx, rsyncNodes, client.MatchingLabels{ownerLabelRsyncNodeDataMovement: dm.Name}); err != nil {
		return nil, err
	}

	log.V(1).Info("Deleting all nodes", "count", len(rsyncNodes.Items))
	for _, node := range rsyncNodes.Items {
		if err := r.Delete(ctx, &node); err != nil {
			if !errors.IsNotFound(err) {
				log.V(1).Info("Deleting", "node", node.Namespace, "error", err.Error())
				return nil, err
			}
		}
	}

	return nil, nil
}

func rsyncNodeDataMovementEnqueueRequestMapFunc(o client.Object) []reconcile.Request {

	if owner, found := o.GetAnnotations()[ownerLabelRsyncNodeDataMovement]; found {
		components := strings.Split(owner, "/")
		if len(components) == 2 {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Name:      components[0],
					Namespace: components[1],
				},
			}}
		}
	}

	return []reconcile.Request{}
}
