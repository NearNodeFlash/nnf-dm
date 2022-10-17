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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/NearNodeFlash/nnf-dm/controllers/metrics"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

const (
	finalizer = "dm.cray.hpe.com"

	InitiatorLabel = "dm.cray.hpe.com/initiator"

	// DM ConfigMap Info
	DMConfigMapName      = "nnf-dm-config"
	DMConfigMapNamespace = corev1.NamespaceDefault

	// DM ConfigMap Data Keys
	DMConfigKeyCmd          = "dmCommand"
	DMConfigKeyProgInterval = "dmProgressInterval"
	DMConfigKeyNumProcesses = "dmNumProcesses"

	// DM ConfigMap Default Values
	DMConfigDefaultCmd          = ""
	DMConfigDefaultProgInterval = 5 * time.Second
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// We maintain a map of active operations which allows us to process cancel requests
	// This is a thread safe map since multiple data movement reconcilers and go routines will be executing at the same time.
	contexts sync.Map
}

type DMConfigMapData map[string]string

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress
type dataMovementCancelContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Invalid error is a non-recoverable error type that implies the Data Movement resource is invalid
type invalidError struct {
	err error
}

func newInvalidError(format string, a ...any) *invalidError {
	return &invalidError{
		err: fmt.Errorf(format, a...),
	}
}

func (i *invalidError) Error() string { return i.err.Error() }
func (i *invalidError) Unwrap() error { return i.err }

// Regex to scrape the progress output of the `dcp` command. Example output:
// Copied 1.000 GiB (10%) in 1.001 secs (4.174 GiB/s) 9 secs left ..."
var progressRe = regexp.MustCompile(`Copied\s(\d+\.\d+)\s(\S{3})\s\((\d{2,3})%\)\sin`)

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts,verbs=get;list
//+kubebuilder:rbac:groups=dws.cray.hpe.com,resources=clientmounts/status,verbs=get;list
//+kubebuilder:rbac:groups=cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	metrics.NnfDmDataMovementReconcilesTotal.Inc()

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !dm.GetDeletionTimestamp().IsZero() {

		if err := r.cancel(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}

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

	// Prevent gratuitous wakeups for a resource that is already finished.
	if dm.Status.State == nnfv1alpha1.DataMovementConditionTypeFinished {
		return ctrl.Result{}, nil
	}

	// Handle cancellation
	if dm.Spec.Cancel {
		if err := r.cancel(ctx, dm); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Make sure if the DM is already running that we don't start up another command
	if dm.Status.State == nnfv1alpha1.DataMovementConditionTypeRunning {

		// If we're currently tracking the resource, then we know for certain the
		// resource is running and there's nothing further we need to do.
		if _, found := r.contexts.Load(dm.Name); found {
			return ctrl.Result{}, nil
		}

		// Otherwise, if we're _not_ tracking the resource, we know the pod restarted
		// and the DM command terminated. In this case we fall-through to restart the
		// data movement operation.
		dm.Status.Restarts += 1
		log.Info("Restarting", "restarts", dm.Status.Restarts)
	}

	// Handle invalid errors that can occur when setting up the data movement
	// resource. An invalid error is unrecoverable.
	handleInvalidError := func(err error) error {
		if errors.Is(err, &invalidError{}) {
			dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
			dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonInvalid
			dm.Status.Message = err.Error()

			if err := r.Status().Update(ctx, dm); err != nil {
				return err
			}

			return nil
		}

		return err
	}

	nodes, err := r.getStorageNodeNames(ctx, dm)
	if err != nil {
		return ctrl.Result{}, handleInvalidError(err)
	}

	hosts, err := r.getWorkerHostnames(ctx, nodes)
	if err != nil {
		return ctrl.Result{}, handleInvalidError(err)
	}

	// Pull configurable values from the ConfigMap
	var dmCmd string
	var progressCollectInterval time.Duration
	var numProcesses int
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: DMConfigMapName, Namespace: DMConfigMapNamespace}, configMap); err != nil {
		log.Info("Config map not found - requeueing")
		return ctrl.Result{Requeue: true}, nil
	} else {
		log.Info("Config map found:", "configMap", configMap, "data", configMap.Data)
		dmCmd, progressCollectInterval, numProcesses = parseConfigMapValues(*configMap, len(hosts))
	}

	// Record the start of the data movement operation
	now := metav1.NowMicro()
	dm.Status.StartTime = &now
	dm.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning
	dm.Status.CommandStatus = &nnfv1alpha1.NnfDataMovementCommandStatus{}
	cmdStatus := nnfv1alpha1.NnfDataMovementCommandStatus{}

	if err := r.Status().Update(ctx, dm); err != nil {
		return ctrl.Result{}, err
	}

	// Expand the context with cancel and store it in the map so the cancel
	// function can be used in another reconciler loop. Also add NamespacedName
	// so we can retrieve the resource.
	ctxCancel, cancel := context.WithCancel(ctx)
	r.contexts.Store(dm.Name, dataMovementCancelContext{
		ctx:    ctxCancel,
		cancel: cancel,
	})

	// Execute the go routine to perform the data movement
	go func() {
		var cmd *exec.Cmd

		// Allow the data movement command to be overridden via the configmapa
		// for testing purposes
		if dmCmd != "" {
			cmdList := strings.Split(dmCmd, " ")
			cmd = exec.CommandContext(ctxCancel, cmdList[0], cmdList[1:]...)
			// Otherwise use the actual `mpirun dcp` command
		} else {
			// TODO: UserId/GroupId
			cmd = exec.CommandContext(ctxCancel,
				"mpirun",
				"--allow-run-as-root",
				"-np", fmt.Sprintf("%d", numProcesses), // # TODO: Might want to adjust this if running on a rabbit node
				"--host", strings.Join(hosts, ","),
				// Dump progress every second to ensure progress collection has
				// something to collect even if the interval is not 1 second.
				"dcp", "--progress", "1", dm.Spec.Source.Path, dm.Spec.Destination.Path,
			)
		}
		cmdStatus.Command = cmd.String()
		log.Info("Running Command", "cmd", cmdStatus.Command)

		// Use a MultiWriter so that we can parse the output and save the full output at the end
		var combinedOutBuf, parseBuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &parseBuf, &combinedOutBuf)
		cmd.Stderr = cmd.Stdout // Combine stderr/stdout

		// Use channels to sync progress collection and cmd.Wait().
		chCommandDone := make(chan bool, 1)
		chProgressDone := make(chan bool)

		// Start the data movement command
		cmd.Start()

		// While the command is running, capture and process the output. Read
		// lines until EOF to ensure we have the latest output. Then use the
		// last regex match to obtain the most recent progress.
		go func() {
			var progressInt32 *int32
			var elapsed metav1.Duration
			elapsed.Duration = 0
			progressStart := metav1.NowMicro()

			// Perform the actual collection and update logic
			parseAndUpdateProgress := func() {
				progressOutput := ""

				// Read all lines of output until EOF
				for {
					line, err := parseBuf.ReadString('\n')
					if err == io.EOF {
						break
					} else if err != nil {
						log.Error(err, "failed to read progress output")
					}
					progressOutput += line
				}

				// Get the times now before we start the parsing
				progressNow := metav1.NowMicro()
				elapsed.Duration = progressNow.Time.Sub(progressStart.Time)

				// Find all matching progress matches but we only care about the
				// last (most recent one)
				groups := progressRe.FindAllStringSubmatch(progressOutput, -1)
				if len(groups) > 0 {
					mostRecent := groups[len(groups)-1]

					// Get the progress and convert to int
					progress, err := strconv.Atoi(mostRecent[3])
					if err == nil {
						// Initialize the pointer if we haven't done so already.
						// We don't want to do this above since we do not want an
						// initial value of 0 unless we actually have a progress
						if progressInt32 == nil {
							progressInt32 = new(int32)
						}
						*progressInt32 = int32(progress)
					} else {
						log.Error(err, "failed to parse progress output", "progressMatch", mostRecent)
					}
				}

				// Update the CommandStatus in the DM resource
				err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					dm := &nnfv1alpha1.NnfDataMovement{}
					if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
						return client.IgnoreNotFound(err)
					}

					// Update progress if we have one
					if progressInt32 != nil {
						cmdStatus.ProgressPercentage = progressInt32
					}
					// Update the output and times regardless
					cmdStatus.LastMessage = progressOutput
					cmdStatus.LastMessageTime = progressNow
					cmdStatus.ElapsedTime = elapsed

					if dm.Status.CommandStatus == nil {
						dm.Status.CommandStatus = &nnfv1alpha1.NnfDataMovementCommandStatus{}
					}
					cmdStatus.DeepCopyInto(dm.Status.CommandStatus)

					log.Info("Updating Progress", "CommandStatus", dm.Status.CommandStatus)
					return r.Status().Update(ctx, dm)
				})

				if err != nil {
					log.Error(err, "failed to update CommandStatus with Progress", "CommandStatus", cmdStatus)
				}
			}

			// Main Progress Collection Loop
			for {
				select {
				// Now that we're done, parse whatever output is left
				case <-chCommandDone:
					parseAndUpdateProgress()
					chProgressDone <- true
					return
				// Collect Progress output on every interval
				case <-time.After(progressCollectInterval):
					parseAndUpdateProgress()
				}
			}
		}()

		err := cmd.Wait()
		chCommandDone <- true // tell the process goroutine to stop parsing output
		<-chProgressDone      // wait for process goroutine to stop parsing final output

		// Command is finished, update status
		now := metav1.NowMicro()
		dm.Status.EndTime = &now
		dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess

		if errors.Is(ctxCancel.Err(), context.Canceled) {
			log.Error(err, "Data movement operation cancelled", "output", combinedOutBuf.String())
			dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonCancelled
		} else if err != nil {
			log.Error(err, "Data movement operation failed", "output", combinedOutBuf.String())
			dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonFailed
			dm.Status.Message = fmt.Sprintf("%s: %s", err.Error(), combinedOutBuf.String())

			// TODO: Enhanced error capture: parse error response and provide useful message
		} else {
			log.Info("Completed Command", "cmdStatus", cmdStatus)
		}

		status := dm.Status.DeepCopy()

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			dm := &nnfv1alpha1.NnfDataMovement{}
			if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
				return client.IgnoreNotFound(err)
			}

			// Ensure we have the latest CommandStatus from the progress goroutine
			cmdStatus.DeepCopyInto(status.CommandStatus)
			status.DeepCopyInto(&dm.Status)

			return r.Status().Update(ctx, dm)
		})

		if err != nil {
			log.Error(err, "failed to update dm status with completion")
			// TODO Add prometheus counter to track occurrences
		}

		r.contexts.Delete(dm.Name)
	}()

	return ctrl.Result{}, nil
}

// Read the DM ConfigMap and attempt to parse the values out of it. If
// everything is parsable, return the values. Otherwise use the defaults.
//
// Returns Data Movement Command, Collect Interval, NumProcesses
func parseConfigMapValues(configMap corev1.ConfigMap, numHosts int) (string, time.Duration, int) {
	dmCmd := DMConfigDefaultCmd
	progressCollectInterval := DMConfigDefaultProgInterval
	numProcesses := numHosts
	var err error

	// Data Movement Command
	if configMap.Data[DMConfigKeyCmd] != "" {
		dmCmd = configMap.Data[DMConfigKeyCmd]
	}

	// Data Movement Progress Collection Interval
	if configMap.Data[DMConfigKeyProgInterval] != "" {
		if progressCollectInterval, err = time.ParseDuration(configMap.Data[DMConfigKeyProgInterval]); err != nil {
			progressCollectInterval = DMConfigDefaultProgInterval
		}
	}

	// Data Movement NumProcesses
	if configMap.Data[DMConfigKeyNumProcesses] != "" {
		if numProcesses, err = strconv.Atoi(configMap.Data[DMConfigKeyNumProcesses]); err != nil {
			numProcesses = numHosts
		}
	}

	return dmCmd, progressCollectInterval, numProcesses
}

func (r *DataMovementReconciler) cancel(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) error {
	log := log.FromContext(ctx)

	// Check for the scenario where a request is canceled but not deleted before the DM has started.
	// If so, record it as cancelled and do nothing more with the data movement operation
	if dm.Status.StartTime.IsZero() && !dm.DeletionTimestamp.IsZero() {
		now := metav1.NowMicro()
		dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonCancelled
		dm.Status.StartTime = &now
		dm.Status.EndTime = &now

		if err := r.Status().Update(ctx, dm); err != nil {
			return err
		}

		log.Info("Cancel initiated before data movement started, doing nothing")
		return nil
	}

	storedCancelContext, found := r.contexts.LoadAndDelete(dm.Name)
	if !found {
		return nil // Already completed or cancelled?
	}

	cancelContext := storedCancelContext.(dataMovementCancelContext)

	log.Info("Cancelling operation")
	cancelContext.cancel()
	<-cancelContext.ctx.Done()

	// Nothing more to do - the go routine that is executing the data movement will exit
	// and the status is recorded then.

	return nil
}

func isTestEnv() bool {
	_, found := os.LookupEnv("NNF_TEST_ENVIRONMENT")
	return found
}

// Retrieve the NNF Nodes that are the target of the data movement operation
func (r *DataMovementReconciler) getStorageNodeNames(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) ([]string, error) {
	// If this is a node data movement request simply reference the localhost
	if dm.Namespace == os.Getenv("NNF_NODE_NAME") || isTestEnv() {
		return []string{"localhost"}, nil
	}

	// Otherwise, this is a system wide data movement request we target the NNF Nodes that are defined in the storage specification
	var storageRef corev1.ObjectReference
	if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Source.StorageReference
	} else if dm.Spec.Destination.StorageReference.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		storageRef = dm.Spec.Destination.StorageReference
	} else {
		return nil, newInvalidError("Neither source or destination is of NNF Storage type")
	}

	storage := &nnfv1alpha1.NnfStorage{}
	if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, newInvalidError("NNF Storage not found: %s", err.Error())
		}
		return nil, err
	}

	if storage.Spec.FileSystemType != "lustre" {
		return nil, newInvalidError("Unsupported storage type %s", storage.Spec.FileSystemType)
	}
	targetAllocationSetIndex := -1
	for allocationSetIndex, allocationSet := range storage.Spec.AllocationSets {
		if allocationSet.TargetType == "OST" {
			targetAllocationSetIndex = allocationSetIndex
		}
	}

	if targetAllocationSetIndex == -1 {
		return nil, newInvalidError("OST allocation set not found")
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
			return nil, newInvalidError("Hostname invalid for node %s", nodes[idx])
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
