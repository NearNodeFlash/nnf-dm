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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

const (
	rsyncSuffix = "-rsync"
)

func (r *DataMovementReconciler) initializeRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {

	if dm.Spec.Source == nil && dm.Spec.Destination == nil {
		return nil, nil
	}

	nodes, err := r.getStorageNodes(ctx, dm)
	if err != nil {
		return nil, err
	}

	return r.startNodeDataMovers(ctx, dm, nodes)
}

func (r *DataMovementReconciler) getStorageNodes(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) ([]nnfv1alpha1.NnfStorageAllocationNodes, error) {

	// Retrieve the NnfAccess for this data movement request. This will reference the NnfStorage object so we can target the nodes
	// that make up the storage for data movement. We currently support data movement from Lustre (global or persistence) to/from
	// the NNF Nodes describe by a single NnfAccess.

	sourceFileSystemType, err := r.getStorageInstanceFileSystemType(ctx, dm.Spec.Source.Storage)
	if err != nil {
		return nil, err
	}

	destinationFileSystemType, err := r.getStorageInstanceFileSystemType(ctx, dm.Spec.Destination.Storage)
	if err != nil {
		return nil, err
	}

	if sourceFileSystemType != "lustre" && destinationFileSystemType != "lustre" {
		return nil, fmt.Errorf("rsync data movement requires one of source or destination to be a lustre file system")
	}

	var accessReference *corev1.ObjectReference = nil
	if sourceFileSystemType != "lustre" {
		accessReference = dm.Spec.Source.Access
	} else if destinationFileSystemType != "lustre" {
		accessReference = dm.Spec.Destination.Access
	}

	if accessReference == nil {
		return nil, fmt.Errorf("no access defined for rsync data movement")
	}

	access := &nnfv1alpha1.NnfAccess{}
	if err := r.Get(ctx, types.NamespacedName{Name: accessReference.Name, Namespace: accessReference.Namespace}, access); err != nil {
		return nil, err
	}

	// Retrieve the NnfStorage object that is associated with this Rsync job. This provides the list of Rabbits that will receive
	// RsyncNodeDataMovement resources.
	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: access.Spec.StorageReference.Name, Namespace: access.Spec.StorageReference.Namespace}, storage); err != nil {
		return nil, err
	}

	// The NnfStorage specification is a list of Allocation Sets; with each set containing a number of Nodes <Name, Count> pair that
	// describes the Rabbit and the number of allocations to perform on that node. Since this is an Rsync job, the expected number
	// of Allocation Sets is one, and it should be of xfs/gfs2 type.
	fileSystemType := storage.Spec.FileSystemType
	if fileSystemType == "xfs" || fileSystemType == "gfs2" {
		if len(storage.Spec.AllocationSets) == 1 {
			return storage.Spec.AllocationSets[0].Nodes, nil
		}
	}

	return nil, fmt.Errorf("Invalid NnfStorage: Must have xfs/gfs2 allocation set. Must have allocation specification")
}

func (r *DataMovementReconciler) startNodeDataMovers(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement, nodes []nnfv1alpha1.NnfStorageAllocationNodes) (*ctrl.Result, error) {
	log := log.FromContext(ctx)

	config, err := r.getDataMovementConfigMap(ctx)
	if err != nil {
		return nil, err
	}

	// From the provided NNF Access, we'll need the source/destination to load the PrefixPath; this is the
	// path that is the basis for this data movement request, and we should append "/compute-%id" onto the prefix
	// path, and then append the rsync.Spec.Source/rsync.Spec.Destination as needed.
	//
	// For example, if a request is for XFS Storage, with Source=file.in Destination=file.out, NnfAccess will contain
	// a prefix path that corresponds to the XFS File System at something like /mnt/nnf/job-1234/. In this case
	// we would create rsync jobs with Destination=/mnt/nnf/job-1234/compute-%id/file.out

	sourceAccess := &nnfv1alpha1.NnfAccess{}
	if dm.Spec.Source.Access != nil {
		if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Source.Access.Name, Namespace: dm.Spec.Source.Access.Namespace}, sourceAccess); err != nil {
			return nil, err
		}
	}

	destinationAccess := &nnfv1alpha1.NnfAccess{}
	if dm.Spec.Destination.Access != nil {
		if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.Destination.Access.Name, Namespace: dm.Spec.Destination.Access.Namespace}, destinationAccess); err != nil {
			return nil, err
		}
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
				},
				Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
					Source:      r.getRsyncPath(dm.Spec.Source, config, sourceAccess.Spec.MountPathPrefix, i, configSourcePath),
					Destination: r.getRsyncPath(dm.Spec.Destination, config, destinationAccess.Spec.MountPathPrefix, i, configDestinationPath),
					UserId:      dm.Spec.UserId,
					GroupId:     dm.Spec.GroupId,
				},
			}

			dwsv1alpha1.InheritParentLabels(rsyncNode, dm)
			dwsv1alpha1.AddOwnerLabels(rsyncNode, dm)

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

func (r *DataMovementReconciler) getRsyncPath(spec *nnfv1alpha1.NnfDataMovementSpecSourceDestination, config *corev1.ConfigMap, prefixPath string, index int, override string) string {
	if path, found := config.Data[override]; found {
		return path
	}

	if spec.Storage == nil {
		return spec.Path
	}
	switch spec.Storage.Kind {
	case reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name():
		return spec.Path
	case reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name():
		return prefixPath + fmt.Sprintf("/%d", index) + spec.Path
	}

	panic(fmt.Sprintf("Unsupported Storage Instance %s", spec.Storage.Kind))
}

func (r *DataMovementReconciler) monitorRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, string, string, error) {

	// Create a map by node name so the status' can be refreshed
	statusMap := map[string]*nnfv1alpha1.NnfDataMovementNodeStatus{}
	for statusIdx, status := range dm.Status.NodeStatus {
		dm.Status.NodeStatus[statusIdx].Complete = 0
		dm.Status.NodeStatus[statusIdx].Running = 0
		dm.Status.NodeStatus[statusIdx].Messages = make([]string, 0)
		statusMap[status.Node] = &dm.Status.NodeStatus[statusIdx]
	}

	// Fetch all the rsync node data movements that are labeled with data movement resource.
	nodes := &dmv1alpha1.RsyncNodeDataMovementList{}
	if err := r.List(ctx, nodes, dwsv1alpha1.MatchingOwner(dm)); err != nil {
		return nil, "", "", err
	}

	// Iterate over all the rsync nodes retrieved and accumulate their status/progress into
	// this data movement resource. After this enumeration, we know the current status of
	// all rsync nodes in operation.
	for _, node := range nodes.Items {

		status, found := statusMap[node.Namespace]
		if !found {
			dm.Status.NodeStatus = append(dm.Status.NodeStatus,
				nnfv1alpha1.NnfDataMovementNodeStatus{
					Count:    0,
					Complete: 0,
					Running:  0,
					Messages: make([]string, 0),
				})

			statusMap[node.Namespace] = &dm.Status.NodeStatus[len(dm.Status.NodeStatus)-1]
			status = &dm.Status.NodeStatus[len(dm.Status.NodeStatus)-1]
		}

		switch node.Status.State {
		case nnfv1alpha1.DataMovementConditionTypeFinished:
			status.Complete++
		default:
			status.Running++
		}

		if len(node.Status.Message) != 0 {
			status.Messages = append(status.Messages, node.Status.Message)
		}
	}

	// Iterate over all the accumulated node status' and set the global state of
	// this data movement operation. Any node which has an error impacts the
	// entire data movement resource. Only the first error is recorded, the rest
	// are available in the individual logs.
	currentStatus := nnfv1alpha1.DataMovementConditionReasonSuccess
	currentMessage := ""
	for _, status := range dm.Status.NodeStatus {

		// Set the state to running if not currently in a failed state. This ensures we report a
		// worse case failed condition above any in progress state.
		if status.Running > 0 && currentStatus != nnfv1alpha1.DataMovementConditionReasonFailed {
			currentStatus = nnfv1alpha1.DataMovementConditionTypeRunning
		}

		if len(status.Messages) != 0 {
			currentStatus = nnfv1alpha1.DataMovementConditionReasonFailed

			if len(currentMessage) == 0 {
				if len(status.Messages) == 1 {
					currentMessage = fmt.Sprintf("Failure detected on node %s: Error: %s", status.Node, status.Messages[0])
				} else {
					currentMessage = fmt.Sprintf("Failure detected on node %s: Multiple Errors: %s", status.Node, strings.Join(status.Messages, ", "))
				}
			} else {
				currentMessage = fmt.Sprintf("Failure detected on multiple nodes")
			}
		}
	}

	return nil, currentStatus, currentMessage, nil

}

func (r *DataMovementReconciler) teardownRsyncJob(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(1).Info("Deleting all nodes")

	deleteStatus, err := dwsv1alpha1.DeleteChildren(ctx, r.Client, []dwsv1alpha1.ObjectList{&dmv1alpha1.RsyncNodeDataMovementList{}}, dm)
	if err != nil {
		return nil, err
	}

	if deleteStatus == dwsv1alpha1.DeleteRetry {
		return &ctrl.Result{}, nil
	}

	return nil, nil
}
