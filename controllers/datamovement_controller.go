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
	"os/exec"
	"reflect"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

const (
	finalizer = "dm.cray.hpe.com"

	InitiatorLabel = "dm.cray.hpe.com/initiator"
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// We maintain a map of active and completed operations which allows us to...
	// 1. Process cancel requests (TODO Work with Blake)
	// 2. Handle resource conflict errors where the operation completes but we fail to update the status.
	//
	// This is a thread safe map since multiple data movement reconcilers routines will be executing at the same time.
	completions sync.Map
}

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfaccesses,verbs=get;list;watch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts,verbs=get;list
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts/status,verbs=get;list
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !dm.GetDeletionTimestamp().IsZero() {

		// TODO: Cancel/Abort the operation from Blake

		r.completions.Delete(req.Name)

		if controllerutil.ContainsFinalizer(dm, finalizer) {
			controllerutil.RemoveFinalizer(dm, finalizer)

			if err := r.Update(ctx, dm); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dm, finalizer) {
		controllerutil.AddFinalizer(dm, finalizer)
		if err := r.Update(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}

		// An update here will cause the reconciler to run again once kubernetes
		// has recorded the resource it its database.
		return ctrl.Result{}, nil
	}

	// Prevent gratuitous wakeups for an resource that is already finished.
	if dm.Status.State == nnfv1alpha1.DataMovementConditionTypeFinished {
		return ctrl.Result{}, nil
	}

	if dm.Status.StartTime.IsZero() {
		now := metav1.NowMicro()
		dm.Status.StartTime = &now
		dm.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning

		if err := r.Status().Update(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}
	}

	// The data movement operation may have completed but we failed to record the completion status
	// due to a resource conflict. Check if the resource completed and retry updating the status.
	if dm.Status.State == nnfv1alpha1.DataMovementConditionTypeRunning {
		status, found := r.completions.Load(req.Name)
		if found {
			status := status.(nnfv1alpha1.NnfDataMovementStatus)

			log.Info("status update restored", "status", fmt.Sprintf("%+v", status))

			status.DeepCopyInto(&dm.Status)

			if err := r.Status().Update(ctx, dm); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	// TODO: Support Blake's Cancel API

	nodes, err := r.getStorageNodeNames(ctx, dm)
	if err != nil {
		return ctrl.Result{}, err // TODO: Filter out runtime errors (retryable) format errors (failures)
	}

	hosts, err := r.getWorkerHostnames(ctx, nodes)
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO: UserId/GroupId

	cmd := exec.CommandContext(ctx,
		"mpirun",
		"--allow-run-as-root",
		"-np", fmt.Sprintf("%d", len(hosts)), // # TODO: Might want to adjust this if running on a rabbit node
		"--host", strings.Join(hosts, ","),
		"dcp", dm.Spec.Source.Path, dm.Spec.Destination.Path)

	log.Info("Running Commands", "cmd", cmd.String())

	// TODO: Capture output as it progresses and add a %complete to the resource

	out, err := cmd.CombinedOutput()

	now := metav1.NowMicro()
	dm.Status.EndTime = &now
	dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
	dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess

	if err != nil {
		dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonFailed
		dm.Status.Message = err.Error()

		// TODO: Enhanced error capture: parse error response and provide useful message

		log.Error(err, "Data movement operation failed", "output", string(out))
	}

	status := dm.Status.DeepCopy()

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dm := &nnfv1alpha1.NnfDataMovement{}
		if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
			return client.IgnoreNotFound(err)
		}

		status.DeepCopyInto(&dm.Status)

		return r.Status().Update(ctx, dm)
	})

	if errors.IsConflict(err) {

		log.Info("Status update conflicted")

		// Record the completion internally so when we requeue we won't
		// restart the operation but update the resource with saved status.
		r.completions.Store(req.Name, dm.Status)

		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// Retrieve the NNF Nodes that are the target of the data movement operation
func (r *DataMovementReconciler) getStorageNodeNames(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) ([]string, error) {

	// If this is a node data movement request (not in the default/dm namespaces) then the target NNF Node is ourselves.
	if dm.Namespace != metav1.NamespaceDefault && dm.Namespace != dmv1alpha1.DataMovementNamespace {
		return []string{"localhost"}, nil
	}

	// Otherwise, this is a system wide data movement request we target the NNF Nodes that are defined in the storage specification
	var storageRef *corev1.ObjectReference
	if dm.Spec.Source.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Source.Storage
	} else if dm.Spec.Destination.Storage.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Destination.Storage
	} else {
		return nil, fmt.Errorf("Neither source or destination is of NNF Storage type")
	}

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

	nodes := storage.Spec.AllocationSets[targetAllocationSetIndex].Nodes
	nodeNames := make([]string, len(nodes))
	for idx := range nodes {
		nodeNames[idx] = nodes[idx].Name
	}

	return nodeNames, nil
}

func (r *DataMovementReconciler) getWorkerHostnames(ctx context.Context, nodes []string) ([]string, error) {

	if nodes[0] == "localhost" {
		return nodes, nil
	}

	// For this first iteration, we need to look up the Pods associated with the MPI workers on each
	// individual rabbit, mapping the nodename to a worker IP address. Since we've set up a headless
	// service matching the subdomain, the worker's IP is used as the DNS name (substituting '-' for '.')
	// following the description here:
	// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-hostname-and-subdomain-fields
	//
	// Ideally this look-up would not be required if the MPI worker pods could have the same hostname
	// as the nodename. There is no straightfoward way for this to happen although it has been raised
	// several times in the k8s community.
	//
	// A couple of ideas on how to support this...
	// 1. Using an initContainer which would get the parent pod and modify the hostname.
	// 2. Not use a DaemonSet to create the MPI worker pods, but do so manually, assigning
	//    the correct hostname to each pod. Right now the daemon set provides scheduling and
	//    pod restarts, and we would lose this feature if we managed the pods individually.

	// Get the Rabbit DM Worker Pods
	listOptions := []client.ListOption{
		client.InNamespace(dmv1alpha1.DataMovementNamespace),
		client.MatchingLabels(map[string]string{
			dmv1alpha1.DataMovementWorkerLabel: "true",
		}),
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, listOptions...); err != nil {
		return nil, err
	}

	nodeNameToHostnameMap := map[string]string{}
	for _, pod := range pods.Items {
		nodeNameToHostnameMap[pod.Spec.NodeName] = strings.ReplaceAll(pod.Status.PodIP, ".", "-") + ".dm." + dmv1alpha1.DataMovementNamespace // TODO: make the subdomain const TODO: use nnf-dm-system const
	}

	hostnames := make([]string, len(nodes))
	for idx := range nodes {

		hostname, found := nodeNameToHostnameMap[nodes[idx]]
		if !found {
			return nil, fmt.Errorf("Hostname invalid for node %s", nodes[idx])
		}

		hostnames[idx] = hostname
	}

	return hostnames, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 128}).
		Complete(r)
}
