/*
 * Copyright 2021-2024 Hewlett Packard Enterprise Development LP
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	"github.com/NearNodeFlash/nnf-dm/internal/controller/metrics"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	"github.com/NearNodeFlash/nnf-sos/pkg/command"
	"github.com/go-logr/logr"
)

const (
	finalizer = "dm.cray.hpe.com"
)

// Regex to scrape the progress output of the `dcp` command. Example output:
// Copied 1.000 GiB (10%) in 1.001 secs (4.174 GiB/s) 9 secs left ..."
var progressRe = regexp.MustCompile(`Copied.+\(([[:digit:]]{1,3})%\)`)

// Regexes to scrape the stats output of the `dcp` command once it's done. Example output:
/*
Copied 18.626 GiB (100%) in 171.434 secs (111.259 MiB/s) done
Copy data: 18.626 GiB (20000000000 bytes)
Copy rate: 111.258 MiB/s (20000000000 bytes in 171.434 seconds)
Syncing data to disk.
Sync completed in 0.017 seconds.
Fixing permissions.
Updated 2 items in 0.003 seconds (742.669 items/sec)
Syncing directory updates to disk.
Sync completed in 0.001 seconds.
Started: Jul-25-2024,16:44:33
Completed: Jul-25-2024,16:47:25
Seconds: 171.458
Items: 2
  Directories: 1
  Files: 1
  Links: 0
Data: 18.626 GiB (20000000000 bytes)
Rate: 111.243 MiB/s (20000000000 bytes in 171.458 seconds)
*/
type statsRegex struct {
	name  string
	regex *regexp.Regexp
}

var dcpStatsRegexes = []statsRegex{
	{"seconds", regexp.MustCompile(`Seconds: ([[:digit:].]+)`)},
	{"items", regexp.MustCompile(`Items: ([[:digit:]]+)`)},
	{"dirs", regexp.MustCompile(`Directories: ([[:digit:]]+)`)},
	{"files", regexp.MustCompile(`Files: ([[:digit:]]+)`)},
	{"links", regexp.MustCompile(`Files: ([[:digit:]]+)`)},
	{"data", regexp.MustCompile(`Data: (.*)`)},
	{"rate", regexp.MustCompile(`Rate: (.*)`)},
}

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *kruntime.Scheme

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
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfdatamovementprofiles,verbs=get;list;watch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfstorages,verbs=get;list;watch
//+kubebuilder:rbac:groups=nnf.cray.hpe.com,resources=nnfnodestorages,verbs=get;list;watch
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

	// Get DM Profile
	profile, err := r.getDMProfile(ctx, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not get profile for data movement").WithError(err).WithMajor()
	}
	log.Info("Using profile", "profile", profile)

	// Create the hostfile. This is needed for preparing the destination and the data movement
	// command itself.
	mpiHostfile, err := createMpiHostfile(profile, hosts, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not create MPI hostfile").WithError(err).WithMajor()
	}
	log.Info("MPI Hostfile preview", "first line", peekMpiHostfile(mpiHostfile))

	// Prepare Destination Directory
	if err = r.prepareDestination(ctx, profile, mpiHostfile, dm, log); err != nil {
		return ctrl.Result{}, err
	}

	// Build command
	cmdArgs, err := buildDMCommand(ctx, profile, mpiHostfile, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha2.NewResourceError("could not create data movement command").WithError(err).WithMajor()
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
		progressCollectInterval := time.Duration(profile.Data.ProgressIntervalSeconds) * time.Second
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
						if err := parseDcpProgress(line, &cmdStatus); err != nil {
							log.Error(err, "failed to parse progress", "line", line)
							return
						}

						// Collect stats only when finished
						if cmdStatus.ProgressPercentage != nil && *cmdStatus.ProgressPercentage >= 100 {
							if err := parseDcpStats(line, &cmdStatus); err != nil {
								log.Error(err, "failed to parse stats", "line", line)
								return
							}
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
			if profile.Data.LogStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.LogStdout) {
				log.Info("Data movement operation output", "output", combinedOutBuf.String())
			}

			// Profile or DM request has enabled storing stdout
			if profile.Data.StoreStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.StoreStdout) {
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

func parseDcpProgress(line string, cmdStatus *nnfv1alpha1.NnfDataMovementCommandStatus) error {
	match := progressRe.FindStringSubmatch(line)
	if len(match) > 0 {
		progress, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("failed to parse progress output: %w", err)
		}
		progressInt32 := int32(progress)
		cmdStatus.ProgressPercentage = &progressInt32
	}

	return nil
}

// Go through the list of dcp stat regexes, parse them, and put them in their appropriate place in cmdStatus
func parseDcpStats(line string, cmdStatus *nnfv1alpha1.NnfDataMovementCommandStatus) error {
	for _, s := range dcpStatsRegexes {
		match := s.regex.FindStringSubmatch(line)
		if len(match) > 0 {
			matched := true // default case will set this to false

			// Each regex is parsed depending on its type (e.g. float, int, string) and then stored
			// in a different place in cmdStatus
			switch s.name {
			case "seconds":
				cmdStatus.Seconds = match[1]
			case "items":
				items, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Items output: %w", err)
				}
				i32 := int32(items)
				cmdStatus.Items = &i32
			case "dirs":
				dirs, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Directories output: %w", err)
				}
				i32 := int32(dirs)
				cmdStatus.Directories = &i32
			case "files":
				files, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Directories output: %w", err)
				}
				i32 := int32(files)
				cmdStatus.Files = &i32
			case "links":
				links, err := strconv.Atoi(match[1])
				if err != nil {
					return fmt.Errorf("failed to parse Links output: %w", err)
				}
				i32 := int32(links)
				cmdStatus.Links = &i32
			case "data":
				cmdStatus.Data = match[1]
			case "rate":
				cmdStatus.Rate = match[1]
			default:
				matched = false // if we got here then nothing happened, so try the next regex
			}

			// if one of the regexes matched, then we're done
			if matched {
				return nil
			}
		}
	}

	return nil
}

func (r *DataMovementReconciler) getDMProfile(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (*nnfv1alpha1.NnfDataMovementProfile, error) {

	var profile *nnfv1alpha1.NnfDataMovementProfile

	if dm.Spec.ProfileReference.Kind != reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name() {
		return profile, fmt.Errorf("invalid NnfDataMovementProfile kind %s", dm.Spec.ProfileReference.Kind)
	}

	profile = &nnfv1alpha1.NnfDataMovementProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dm.Spec.ProfileReference.Name,
			Namespace: dm.Spec.ProfileReference.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(profile), profile); err != nil {
		return nil, err
	}

	return profile, nil
}

func buildDMCommand(ctx context.Context, profile *nnfv1alpha1.NnfDataMovementProfile, hostfile string, dm *nnfv1alpha1.NnfDataMovement) ([]string, error) {
	log := log.FromContext(ctx)
	userConfig := dm.Spec.UserConfig != nil

	// If Dryrun is enabled, just use the "true" command
	if userConfig && dm.Spec.UserConfig.Dryrun {
		log.Info("Dry run detected")
		return []string{"true"}, nil
	}

	cmd := profile.Data.Command
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
						"command", profile.Data.Command, "DCPOptions", opts)
				}
			} else {
				log.Info("spec.config.dpcOptions is set but no dcp command found in the DM command",
					"command", profile.Data.Command, "DCPOptions", opts)
			}
		}
	}

	return strings.Split(cmd, " "), nil
}

func (r *DataMovementReconciler) prepareDestination(ctx context.Context, profile *nnfv1alpha1.NnfDataMovementProfile, mpiHostfile string, dm *nnfv1alpha1.NnfDataMovement, log logr.Logger) error {
	// These functions interact with the filesystem, so they can't run in the test env
	// Also, if the profile disables destination creation, skip it
	if isTestEnv() || !profile.Data.CreateDestDir {
		return nil
	}

	// Determine the destination directory based on the source and path
	log.Info("Determining destination directory based on source/dest file types")
	destDir, err := getDestinationDir(dm, mpiHostfile, log)
	if err != nil {
		return dwsv1alpha2.NewResourceError("could not determine source type").WithError(err).WithFatal()
	}

	// See if an index mount directory on the destination is required
	log.Info("Determining if index mount directory is required")
	indexMount, err := r.checkIndexMountDir(ctx, dm)
	if err != nil {
		return dwsv1alpha2.NewResourceError("could not determine index mount directory").WithError(err).WithFatal()
	}

	// Account for index mount directory on the destDir and the dm dest path
	// This updates the destination on dm
	if indexMount != "" {
		log.Info("Index mount directory is required", "indexMountdir", indexMount)
		d, err := handleIndexMountDir(destDir, indexMount, dm, mpiHostfile, log)
		if err != nil {
			return dwsv1alpha2.NewResourceError("could not handle index mount directory").WithError(err).WithFatal()
		}
		destDir = d
		log.Info("Updated destination for index mount directory", "destDir", destDir, "dm.Spec.Destination.Path", dm.Spec.Destination.Path)
	}

	// Create the destination directory
	log.Info("Creating destination directory", "destinationDir", destDir, "indexMountDir", indexMount)
	if err := createDestinationDir(destDir, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log); err != nil {
		return dwsv1alpha2.NewResourceError("could not create destination directory").WithError(err).WithFatal()
	}

	log.Info("Destination prepared", "dm.Spec.Destination", dm.Spec.Destination)
	return nil

}

// Check for a copy_out situation by looking at the source filesystem's type. If it's gfs2 or xfs,
// then we need to account for a Fan-In situation and create index mount directories on the
// destination. Returns the index mount directory from the source path.
func (r *DataMovementReconciler) checkIndexMountDir(ctx context.Context, dm *nnfv1alpha1.NnfDataMovement) (string, error) {
	var storage *nnfv1alpha1.NnfStorage
	var nodeStorage *nnfv1alpha1.NnfNodeStorage

	if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name() {
		// The source storage reference is NnfStorage - this came from copy_in/copy_out directives
		storageRef := dm.Spec.Source.StorageReference

		storage = &nnfv1alpha1.NnfStorage{}
		if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, storage); err != nil {
			if apierrors.IsNotFound(err) {
				return "", newInvalidError("could not retrieve NnfStorage for checking index mounts: %s", err.Error())
			}
			return "", err
		}
	} else if dm.Spec.Source.StorageReference.Kind == reflect.TypeOf(nnfv1alpha1.NnfNodeStorage{}).Name() {
		// The source storage reference is NnfNodeStorage - this came from copy_offload
		storageRef := dm.Spec.Source.StorageReference

		nodeStorage = &nnfv1alpha1.NnfNodeStorage{}
		if err := r.Get(ctx, types.NamespacedName{Name: storageRef.Name, Namespace: storageRef.Namespace}, nodeStorage); err != nil {
			if apierrors.IsNotFound(err) {
				return "", newInvalidError("could not retrieve NnfNodeStorage for checking index mounts: %s", err.Error())
			}
			return "", err
		}
	} else {
		return "", nil // nothing to do here
	}

	// If it is gfs2 or xfs, then there are index mounts
	if storage != nil && (storage.Spec.FileSystemType == "gfs2" || storage.Spec.FileSystemType == "xfs") {
		return extractIndexMountDir(dm.Spec.Source.Path, dm.Namespace)
	} else if nodeStorage != nil && (nodeStorage.Spec.FileSystemType == "gfs2" || nodeStorage.Spec.FileSystemType == "xfs") {
		return extractIndexMountDir(dm.Spec.Source.Path, dm.Namespace)
	}

	return "", nil // nothing to do here
}

// Pull out the index mount directory from the path for the correct file systems that require it
func extractIndexMountDir(path, namespace string) (string, error) {

	// To get the index mount directory, We need to scrape the source path to find the index mount
	// directory - we don't have access to the directive index and only know the source/dest paths.
	// Match namespace-digit and then the end of the string OR slash
	pattern := regexp.MustCompile(fmt.Sprintf(`(%s-\d+)(/|$)`, namespace))
	match := pattern.FindStringSubmatch(path)
	if match == nil {
		return "", fmt.Errorf("could not extract index mount directory from source path: %s", path)
	}

	idxMount := match[1]
	// If the path ends with the index mount (and no trailing slash), then we need to return "" so
	// that we don't double up. This happens when you copy out of the root without a trailing slash
	// - dcp will copy the directory (index mount) over to the destination. So there's no need to
	// append it.
	if strings.HasSuffix(path, idxMount) {
		return "", nil // nothing to do here
	}

	return idxMount, nil
}

// Given a destination directory and index mount directory, apply the necessary changes to the
// destination directory and the DM's destination path to account for index mount directories
func handleIndexMountDir(destDir, indexMount string, dm *nnfv1alpha1.NnfDataMovement, mpiHostfile string, log logr.Logger) (string, error) {

	// For cases where the root directory (e.g. $DW_JOB_my_workflow) is supplied, the path will end
	// in the index mount directory without a trailing slash. If that's the case, there's nothing
	// to handle.
	if strings.HasSuffix(dm.Spec.Source.Path, indexMount) {
		return destDir, nil
	}

	// Add index mount directory to the end of the destination directory
	idxMntDir := filepath.Join(destDir, indexMount)

	// Update dm.Spec.Destination.Path for the new path
	dmDest := idxMntDir

	// Get the source to see if it's a file
	srcIsFile, err := isSourceAFile(dm.Spec.Source.Path, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log)
	if err != nil {
		return "", err
	}
	destIsFile := isDestAFile(dm.Spec.Destination.Path, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log)
	log.Info("Checking source and dest types", "srcIsFile", srcIsFile, "destIsFile", destIsFile)

	// Account for file-file
	if srcIsFile && destIsFile {
		dmDest = filepath.Join(idxMntDir, filepath.Base(dm.Spec.Destination.Path))
	}

	// Preserve the trailing slash. This should not matter in dcp's eye, but let's preserve it for
	// posterity.
	if strings.HasSuffix(dm.Spec.Destination.Path, "/") {
		dmDest += "/"
	}

	// Update the dm destination with the new path
	dm.Spec.Destination.Path = dmDest

	return idxMntDir, nil
}

// Determine the directory path to create based on the source and destination.
// Returns the mkdir directory and error.
func getDestinationDir(dm *nnfv1alpha1.NnfDataMovement, mpiHostfile string, log logr.Logger) (string, error) {
	// Default to using the full path of dest
	src := dm.Spec.Source.Path
	dest := dm.Spec.Destination.Path
	destDir := dest

	srcIsFile, err := isSourceAFile(src, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log)
	if err != nil {
		return "", err
	}

	// Account for file-file data movement - we don't want the full path
	// ex: /path/to/a/file -> /path/to/a
	if srcIsFile && isDestAFile(dest, dm.Spec.UserId, dm.Spec.GroupId, mpiHostfile, log) {
		destDir = filepath.Dir(dest)
	}

	// We know it's a directory, so we don't care about the trailing slash
	return filepath.Clean(destDir), nil
}

// Use mpirun to run stat on a file as a given UID/GID by using `setpriv`
func mpiStat(path string, uid, gid uint32, mpiHostfile string, log logr.Logger) (string, error) {
	// Use setpriv to stat the path with the specified UID/GID. Only run it on 1 host (ie. -n 1)
	cmd := fmt.Sprintf("mpirun --allow-run-as-root -n 1 --hostfile %s -- setpriv --euid %d --egid %d --clear-groups stat --cached never -c '%%F' %s",
		mpiHostfile, uid, gid, path)

	output, err := command.Run(cmd, log)
	if err != nil {
		return output, fmt.Errorf("could not stat path ('%s'): %w", path, err)
	}
	log.Info("mpiStat", "path", path, "output", output)

	return output, nil
}

// Use mpirun to check if a file exists
func mpiExists(path string, uid, gid uint32, mpiHostfile string, log logr.Logger) (bool, error) {
	_, err := mpiStat(path, uid, gid, mpiHostfile, log)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Use mpirun to determine if a given path is a directory
func mpiIsDir(path string, uid, gid uint32, mpiHostfile string, log logr.Logger) (bool, error) {
	output, err := mpiStat(path, uid, gid, mpiHostfile, log)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(output), "directory") {
		log.Info("mpiIsDir", "directory", true)
		return true, nil
	} else {
		log.Info("mpiIsDir", "directory", true)
		return false, nil
	}
}

// Check to see if the source path is a file. The source must exist and will result in error if it
// does not. Do not use mpi in the test environment.
func isSourceAFile(src string, uid, gid uint32, mpiHostFile string, log logr.Logger) (bool, error) {
	var isDir bool
	var err error

	if isTestEnv() {
		sf, err := os.Stat(src)
		if err != nil {
			return false, fmt.Errorf("source file does not exist: %w", err)
		}
		isDir = sf.IsDir()
	} else {
		isDir, err = mpiIsDir(src, uid, gid, mpiHostFile, log)
		if err != nil {
			return false, err
		}
	}

	return !isDir, nil
}

// Check to see if the destination path is a file. If it exists, use stat to determine if it is a
// file or a directory. If it doesn't exist, check for a trailing slash and make an assumption based
// on that.
func isDestAFile(dest string, uid, gid uint32, mpiHostFile string, log logr.Logger) bool {
	isFile := false
	exists := true

	// Attempt to the get the destination file. The error is not important since an assumption can
	// be made by checking if there is a trailing slash.
	if isTestEnv() {
		df, _ := os.Stat(dest)
		if df != nil { // file exists, so use IsDir()
			isFile = !df.IsDir()
		} else {
			exists = false
		}
	} else {
		isDir, err := mpiIsDir(dest, uid, gid, mpiHostFile, log)
		if err == nil { // file exists, so use mpiIsDir() result
			isFile = !isDir
		} else {
			exists = false
		}
	}

	// Dest does not exist but looks like a file (no trailing slash)
	if !exists && !strings.HasSuffix(dest, "/") {
		log.Info("Destination does not exist and has no trailing slash - assuming file", "dest", dest)
		isFile = true
	}

	return isFile
}

func createDestinationDir(dest string, uid, gid uint32, mpiHostfile string, log logr.Logger) error {
	// Don't do anything if it already exists
	if exists, err := mpiExists(dest, uid, gid, mpiHostfile, log); err != nil {
		return err
	} else if exists {
		return nil
	}

	// Use setpriv to create the directory with the specified UID/GID
	cmd := fmt.Sprintf("mpirun --allow-run-as-root --hostfile %s -- setpriv --euid %d --egid %d --clear-groups mkdir -p %s",
		mpiHostfile, uid, gid, dest)
	output, err := command.Run(cmd, log)
	if err != nil {
		return fmt.Errorf("data movement mkdir failed ('%s'): %w output: %s", cmd, err, output)
	}

	return nil
}

// Create an MPI hostfile given settings from a profile and user config from the dm
func createMpiHostfile(profile *nnfv1alpha1.NnfDataMovementProfile, hosts []string, dm *nnfv1alpha1.NnfDataMovement) (string, error) {
	userConfig := dm.Spec.UserConfig != nil

	// Create MPI hostfile only if included in the provided command
	slots := profile.Data.Slots
	maxSlots := profile.Data.MaxSlots

	// Allow the user to override the slots and max_slots in the hostfile.
	if userConfig && dm.Spec.UserConfig.Slots != nil && *dm.Spec.UserConfig.Slots >= 0 {
		slots = *dm.Spec.UserConfig.Slots
	}
	if userConfig && dm.Spec.UserConfig.MaxSlots != nil && *dm.Spec.UserConfig.MaxSlots >= 0 {
		maxSlots = *dm.Spec.UserConfig.MaxSlots
	}

	// Create it
	hostfile, err := writeMpiHostfile(dm.Name, hosts, slots, maxSlots)
	if err != nil {
		return "", err
	}

	return hostfile, nil
}

// Create the MPI Hostfile given a list of hosts, slots, and maxSlots. A temporary directory is
// created based on the DM Name. The hostfile is created inside of this directory.
// A value of 0 for slots or maxSlots will not use it in the hostfile.
func writeMpiHostfile(dmName string, hosts []string, slots, maxSlots int) (string, error) {

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
		if allocationSet.TargetType == "ost" {
			targetAllocationSetIndex = allocationSetIndex
		}
	}

	if targetAllocationSetIndex == -1 {
		return nil, newInvalidError("ost allocation set not found")
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
		client.InNamespace(nnfv1alpha1.DataMovementNamespace),
		client.MatchingLabels(map[string]string{
			nnfv1alpha1.DataMovementWorkerLabel: "true",
		}),
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, listOptions...); err != nil {
		return nil, err
	}

	nodeNameToHostnameMap := map[string]string{}
	for _, pod := range pods.Items {
		nodeNameToHostnameMap[pod.Spec.NodeName] = strings.ReplaceAll(pod.Status.PodIP, ".", "-") + ".dm." + nnfv1alpha1.DataMovementNamespace // TODO: make the subdomain const TODO: use nnf-dm-system const
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
	maxReconciles := runtime.GOMAXPROCS(0)
	return ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxReconciles}).
		WithEventFilter(filterByNamespace(r.WatchNamespace)).
		Complete(r)
}
