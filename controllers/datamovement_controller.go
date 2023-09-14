/*
 * Copyright 2021-2023 Hewlett Packard Enterprise Development LP
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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/yaml"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	"github.com/NearNodeFlash/nnf-dm/controllers/metrics"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

const (
	finalizer = "dm.cray.hpe.com"

	InitiatorLabel = "dm.cray.hpe.com/initiator"

	// DM ConfigMap Info
	configMapName      = "nnf-dm-config"
	configMapNamespace = nnfv1alpha1.DataMovementNamespace

	// DM ConfigMap Data Keys
	configMapKeyData           = "nnf-dm-config.yaml"
	configMapKeyProfileDefault = "default"
)

// Regex to scrape the progress output of the `dcp` command. Example output:
// Copied 1.000 GiB (10%) in 1.001 secs (4.174 GiB/s) 9 secs left ..."
var progressRe = regexp.MustCompile(`Copied.+\(([[:digit:]]{1,3})%\)`)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// We maintain a map of active operations which allows us to process cancel requests
	// This is a thread safe map since multiple data movement reconcilers and go routines will be executing at the same time.
	contexts sync.Map

	WatchNamespace string
}

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress
type dataMovementCancelContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Configuration that matches the nnf-dm-config ConfigMap
type dmConfig struct {
	Profiles                map[string]dmConfigProfile `yaml:"profiles"`
	ProgressIntervalSeconds int                        `yaml:"progressIntervalSeconds,omitempty"`
}

// Each profile can have different settings
type dmConfigProfile struct {
	Slots       int    `yaml:"slots,omitempty"`
	MaxSlots    int    `yaml:"maxSlots,omitempty"`
	Command     string `yaml:"command"`
	LogStdout   bool   `yaml:"logStdout"`
	StoreStdout bool   `yaml:"storeStdout"`
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

//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovements/finalizers,verbs=update
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=dataworkflowservices.github.io,resources=clientmounts,verbs=get;list
//+kubebuilder:rbac:groups=dataworkflowservices.github.io,resources=clientmounts/status,verbs=get;list
//+kubebuilder:rbac:groups=lus.cray.hpe.com,resources=lustrefilesystems,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := log.FromContext(ctx)

	metrics.NnfDmDataMovementReconcilesTotal.Inc()

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		if err != nil {
			resourceError, ok := err.(*dwsv1alpha2.ResourceErrorInfo)
			if ok {
				if resourceError.Severity != dwsv1alpha2.SeverityMinor {
					dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
					dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonInvalid
				}
			}
			dm.Status.SetResourceErrorAndLog(err, log)
			dm.Status.Message = err.Error()

			if updateErr := r.Status().Update(ctx, dm); updateErr != nil {
				err = updateErr
			}
		}
	}()

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
			return ctrl.Result{}, dwsv1alpha2.NewResourceError("").WithError(err).WithUserMessage("Unable to cancel data movement")
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

	nodes, err := r.getStorageNodeNames(ctx, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not get storage nodes for data movement").WithError(err).WithMajor()
	}

	hosts, err := r.getWorkerHostnames(ctx, nodes)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not get worker nodes for data movement").WithError(err).WithMajor()
	}

	// Expand the context with cancel and store it in the map so the cancel function can be used in
	// another reconciler loop. Also add NamespacedName so we can retrieve the resource.
	ctxCancel, cancel := context.WithCancel(ctx)
	r.contexts.Store(dm.Name, dataMovementCancelContext{
		ctx:    ctxCancel,
		cancel: cancel,
	})

	// Get DM Config map
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: configMapNamespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(configMap), configMap); err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not get data movement config map: %v", client.ObjectKeyFromObject(configMap)).WithError(err).WithMajor()
	}

	cfg := dmConfig{}
	if err := yaml.Unmarshal([]byte(configMap.Data[configMapKeyData]), &cfg); err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("invalid data for config map: %v", client.ObjectKeyFromObject(configMap)).WithError(err).WithFatal()
	}
	log.Info("Using config map", "config", cfg)

	// TODO: Allow use of non-default dm config profiles - for now only use the default. For copy
	// offload API, we could create "fake" profiles and store those in the DM object based on the
	// parameters supplied to the CreateRequest().
	// Ensure profile exists
	profile, found := cfg.Profiles[configMapKeyProfileDefault]
	if !found {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("").WithUserMessage("'%s' profile not found in config map: %v", configMapKeyProfileDefault, client.ObjectKeyFromObject(configMap)).WithUser().WithFatal()
	}
	log.Info("Using profile", "name", configMapKeyProfileDefault, "profile", profile)

	cmdArgs, mpiHostfile, err := buildDMCommand(ctx, profile, hosts, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not create data movement command").WithError(err).WithMajor()
	}
	if len(mpiHostfile) > 0 {
		log.Info("MPI Hostfile preview", "first line", peekMpiHostfile(mpiHostfile))
	}

	cmd := exec.CommandContext(ctxCancel, "/bin/bash", "-c", strings.Join(cmdArgs, " "))

	// Record the start of the data movement operation
	now := metav1.NowMicro()
	dm.Status.StartTime = &now
	dm.Status.State = nnfv1alpha1.DataMovementConditionTypeRunning
	cmdStatus := nnfv1alpha1.NnfDataMovementCommandStatus{}
	cmdStatus.Command = cmd.String()
	dm.Status.CommandStatus = &cmdStatus
	log.Info("Running Command", "cmd", cmdStatus.Command)

	if err := r.Status().Update(ctx, dm); err != nil {
		return ctrl.Result{}, err
	}

	// Execute the go routine to perform the data movement
	go func() {
		// Use a MultiWriter so that we can parse the output and save the full output at the end
		var combinedOutBuf, parseBuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(&parseBuf, &combinedOutBuf)
		cmd.Stderr = cmd.Stdout // Combine stderr/stdout

		// Use channels to sync progress collection and cmd.Wait().
		chCommandDone := make(chan bool, 1)
		chProgressDone := make(chan bool)

		// Start the data movement command
		cmd.Start()

		// While the command is running, capture and process the output. Read lines until EOF to
		// ensure we have the latest output. Then use the last regex match to obtain the most recent
		// progress.
		progressCollectInterval := time.Duration(cfg.ProgressIntervalSeconds) * time.Second
		if progressCollectionEnabled(progressCollectInterval) {
			go func() {
				var elapsed metav1.Duration
				elapsed.Duration = 0
				progressStart := metav1.NowMicro()

				// Perform the actual collection and update logic
				parseAndUpdateProgress := func() {

					// Read all lines of output until EOF
					for {
						line, err := parseBuf.ReadString('\n')
						if err == io.EOF {
							break
						} else if err != nil {
							log.Error(err, "failed to read progress output")
						}

						// If it's a progress line, grab the percentage
						match := progressRe.FindStringSubmatch(line)
						if len(match) > 0 {
							progress, err := strconv.Atoi(match[1])
							if err != nil {
								log.Error(err, "failed to parse progress output", "match", match)
								return
							}
							progressInt32 := int32(progress)
							cmdStatus.ProgressPercentage = &progressInt32
						}

						// Always update LastMessage and timing
						cmdStatus.LastMessage = line
						progressNow := metav1.NowMicro()
						elapsed.Duration = progressNow.Time.Sub(progressStart.Time)
						cmdStatus.LastMessageTime = progressNow
						cmdStatus.ElapsedTime = elapsed
					}

					// Update the CommandStatus in the DM resource after we parsed all the lines
					err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
						dm := &nnfv1alpha1.NnfDataMovement{}
						if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
							return client.IgnoreNotFound(err)
						}

						if dm.Status.CommandStatus == nil {
							dm.Status.CommandStatus = &nnfv1alpha1.NnfDataMovementCommandStatus{}
						}
						cmdStatus.DeepCopyInto(dm.Status.CommandStatus)

						return r.Status().Update(ctx, dm)
					})

					if err != nil {
						log.Error(err, "failed to update CommandStatus with Progress", "cmdStatus", cmdStatus)
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
		} else {
			log.Info("Skipping progress collection - collection interval is less than 1s", "collectInterval", progressCollectInterval)
		}

		err := cmd.Wait()

		// If enabled, wait for final progress collection
		if progressCollectionEnabled(progressCollectInterval) {
			chCommandDone <- true // tell the process goroutine to stop parsing output
			<-chProgressDone      // wait for process goroutine to stop parsing final output
		}

		// Command is finished, update status
		now := metav1.NowMicro()
		dm.Status.EndTime = &now
		dm.Status.State = nnfv1alpha1.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonSuccess

		// On cancellation or failure, log the output. On failure, also store the output in the
		// Status.Message. When successful, check the profile/UserConfig config options to log
		// and/or store the output.
		if errors.Is(ctxCancel.Err(), context.Canceled) {
			log.Error(err, "Data movement operation cancelled", "output", combinedOutBuf.String())
			dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonCancelled
		} else if err != nil {
			log.Error(err, "Data movement operation failed", "output", combinedOutBuf.String())
			dm.Status.Status = nnfv1alpha1.DataMovementConditionReasonFailed
			dm.Status.Message = fmt.Sprintf("%s: %s", err.Error(), combinedOutBuf.String())
			resourceErr := dwsv1alpha2.NewResourceError("").WithError(err).WithUserMessage("data movement operation failed: %s", combinedOutBuf.String()).WithFatal()
			dm.Status.SetResourceErrorAndLog(resourceErr, log)
		} else {
			log.Info("Data movement operation completed", "cmdStatus", cmdStatus)

			// Profile or DM request has enabled stdout logging
			if profile.LogStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.LogStdout) {
				log.Info("Data movement operation output", "output", combinedOutBuf.String())
			}

			// Profile or DM request has enabled storing stdout
			if profile.StoreStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.StoreStdout) {
				dm.Status.Message = combinedOutBuf.String()
			}
		}

		os.RemoveAll(filepath.Dir(mpiHostfile))

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

func buildDMCommand(ctx context.Context, profile dmConfigProfile, hosts []string, dm *nnfv1alpha1.NnfDataMovement) ([]string, string, error) {
	log := log.FromContext(ctx)
	var hostfile string
	var err error
	userConfig := dm.Spec.UserConfig != nil

	// If Dryrun is enabled, just use the "true" command with no hostfile
	if userConfig && dm.Spec.UserConfig.Dryrun {
		log.Info("Dry run detected")
		return []string{"true"}, "", nil
	}

	// Command provided via the configmap
	providedCmd := profile.Command

	// Create MPI hostfile only if included in the provided command
	if strings.Contains(providedCmd, "$HOSTFILE") {
		slots := profile.Slots
		maxSlots := profile.MaxSlots

		// Allow the user to override the slots and max_slots in the hostfile.
		if userConfig && dm.Spec.UserConfig.Slots != nil && *dm.Spec.UserConfig.Slots >= 0 {
			slots = *dm.Spec.UserConfig.Slots
		}
		if userConfig && dm.Spec.UserConfig.MaxSlots != nil && *dm.Spec.UserConfig.MaxSlots >= 0 {
			maxSlots = *dm.Spec.UserConfig.MaxSlots
		}

		hostfile, err = createMpiHostfile(dm.Name, hosts, slots, maxSlots)
		if err != nil {
			return nil, "", fmt.Errorf("error creating MPI hostfile: %v", err)
		}
	}

	cmd := profile.Command
	cmd = strings.ReplaceAll(cmd, "$HOSTFILE", hostfile)
	cmd = strings.ReplaceAll(cmd, "$UID", fmt.Sprintf("%d", dm.Spec.UserId))
	cmd = strings.ReplaceAll(cmd, "$GID", fmt.Sprintf("%d", dm.Spec.GroupId))
	cmd = strings.ReplaceAll(cmd, "$SRC", dm.Spec.Source.Path)
	cmd = strings.ReplaceAll(cmd, "$DEST", dm.Spec.Destination.Path)

	// Allow the user to override settings
	if userConfig {

		// Add extra DCP options from the user
		if len(dm.Spec.UserConfig.DCPOptions) > 0 {
			opts := dm.Spec.UserConfig.DCPOptions

			// Insert the extra dcp options before the src argument
			if strings.Contains(cmd, "dcp") {
				idx := strings.Index(cmd, dm.Spec.Source.Path)
				if idx != -1 {
					cmd = cmd[:idx] + opts + " " + cmd[idx:]
				} else {
					log.Info("spec.config.dpcOptions is set but no source path is found in the DM command",
						"command", profile.Command, "DCPOptions", opts)
				}
			} else {
				log.Info("spec.config.dpcOptions is set but no dcp command found in the DM command",
					"command", profile.Command, "DCPOptions", opts)
			}
		}
	}

	return strings.Split(cmd, " "), hostfile, nil
}

// Create an MPI Hostfile given a list of hosts, slots, and maxSlots. A temporary directory is
// created based on the DM Name. The hostfile is created inside of this directory.
// A value of 0 for slots or maxSlots will not use it in the hostfile.
func createMpiHostfile(dmName string, hosts []string, slots, maxSlots int) (string, error) {

	tmpdir := filepath.Join("/tmp", dmName)
	if err := os.MkdirAll(tmpdir, 0755); err != nil {
		return "", err
	}
	hostfilePath := filepath.Join(tmpdir, "hostfile")

	f, err := os.Create(hostfilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// $ cat my-hosts
	// node0 slots=2 max_slots=20
	// node1 slots=2 max_slots=20
	// https://www.open-mpi.org/faq/?category=running#mpirun-hostfile
	for _, host := range hosts {
		if _, err := f.WriteString(host); err != nil {
			return "", err
		}

		if slots > 0 {
			_, err := f.WriteString(fmt.Sprintf(" slots=%d", slots))
			if err != nil {
				return "", err
			}
		}

		if maxSlots > 0 {
			_, err := f.WriteString(fmt.Sprintf(" max_slots=%d", maxSlots))
			if err != nil {
				return "", err
			}
		}

		_, err = f.WriteString("\n")
		if err != nil {
			return "", err
		}
	}

	return hostfilePath, nil
}

// Get the first line of the hostfile for verification
func peekMpiHostfile(hostfile string) string {
	file, err := os.Open(hostfile)
	if err != nil {
		return ""
	}
	defer file.Close()

	str, err := bufio.NewReader(file).ReadString('\n')
	if err != nil {
		return ""
	}

	return str
}

func progressCollectionEnabled(collectInterval time.Duration) bool {
	return collectInterval >= 1*time.Second
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

func filterByNamespace(namespace string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == namespace
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMovementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 128}).
		WithEventFilter(filterByNamespace(r.WatchNamespace)).
		Complete(r)
}
