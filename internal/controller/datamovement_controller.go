/*
 * Copyright 2021-2025 Hewlett Packard Enterprise Development LP
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dwsv1alpha3 "github.com/DataWorkflowServices/dws/api/v1alpha3"
	. "github.com/NearNodeFlash/nnf-dm/internal/controller/helpers"
	"github.com/NearNodeFlash/nnf-dm/internal/controller/metrics"
	nnfv1alpha7 "github.com/NearNodeFlash/nnf-sos/api/v1alpha7"
)

const (
	finalizer = "dm.cray.hpe.com"
)

// DataMovementReconciler reconciles a DataMovement object
type DataMovementReconciler struct {
	client.Client
	Scheme *kruntime.Scheme

	// We maintain a map of active operations which allows us to process cancel requests
	// This is a thread safe map since multiple data movement reconcilers and go routines will be executing at the same time.
	contexts sync.Map

	WatchNamespace string
}

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

	dm := &nnfv1alpha7.NnfDataMovement{}
	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		if err != nil {
			resourceError, ok := err.(*dwsv1alpha3.ResourceErrorInfo)
			if ok {
				if resourceError.Severity != dwsv1alpha3.SeverityMinor {
					dm.Status.State = nnfv1alpha7.DataMovementConditionTypeFinished
					dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonInvalid
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
	if dm.Status.State == nnfv1alpha7.DataMovementConditionTypeFinished {
		return ctrl.Result{}, nil
	}

	// Handle cancellation
	if dm.Spec.Cancel {
		if err := r.cancel(ctx, dm); err != nil {
			return ctrl.Result{}, dwsv1alpha3.NewResourceError("").WithError(err).WithUserMessage("Unable to cancel data movement")
		}

		return ctrl.Result{}, nil
	}

	// Make sure if the DM is already running that we don't start up another command
	if dm.Status.State == nnfv1alpha7.DataMovementConditionTypeRunning {

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

	// Get DM Profile
	profile, err := GetDMProfile(r.Client, ctx, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha3.NewResourceError("could not get profile for data movement").WithError(err).WithMajor()
	}
	log.Info("Using profile", "profile", profile)

	nodes, err := GetStorageNodeNames(r.Client, ctx, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha3.NewResourceError("could not get storage nodes for data movement").WithError(err).WithMajor()
	}

	hosts, err := GetWorkerHostnames(r.Client, ctx, nodes)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha3.NewResourceError("could not get worker nodes for data movement").WithError(err).WithMajor()
	}

	// Expand the context with cancel and store it in the map so the cancel function can be used in
	// another reconciler loop. Also add NamespacedName so we can retrieve the resource.
	ctxCancel, cancel := context.WithCancel(ctx)
	r.contexts.Store(dm.Name, DataMovementCancelContext{
		Ctx:    ctxCancel,
		Cancel: cancel,
	})

	// Create the hostfile. This is needed for preparing the destination and the data movement
	// command itself.
	mpiHostfile, err := CreateMpiHostfile(profile, hosts, dm)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha3.NewResourceError("could not create MPI hostfile").WithError(err).WithMajor()
	}
	log.Info("MPI Hostfile preview", "first line", PeekMpiHostfile(mpiHostfile))

	// Prepare Destination Directory
	if err = PrepareDestination(r.Client, ctx, profile, dm, mpiHostfile, log); err != nil {
		return ctrl.Result{}, err
	}

	// Build command
	cmdArgs, err := BuildDMCommand(profile, mpiHostfile, true, dm, log)
	if err != nil {
		return ctrl.Result{}, dwsv1alpha3.NewResourceError("could not create data movement command").WithError(err).WithMajor()
	}
	cmd := exec.CommandContext(ctxCancel, "/bin/bash", "-c", strings.Join(cmdArgs, " "))

	// Record the start of the data movement operation
	now := metav1.NowMicro()
	dm.Status.StartTime = &now
	dm.Status.State = nnfv1alpha7.DataMovementConditionTypeRunning
	cmdStatus := nnfv1alpha7.NnfDataMovementCommandStatus{}
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
		if ProgressCollectionEnabled(progressCollectInterval) {
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
						if err := ParseDcpProgress(line, &cmdStatus); err != nil {
							log.Error(err, "failed to parse progress", "line", line)
							return
						}

						// Collect stats only when finished
						if cmdStatus.ProgressPercentage != nil && *cmdStatus.ProgressPercentage >= 100 {
							if err := ParseDcpStats(line, &cmdStatus); err != nil {
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
						dm := &nnfv1alpha7.NnfDataMovement{}
						if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
							return client.IgnoreNotFound(err)
						}

						if dm.Status.CommandStatus == nil {
							dm.Status.CommandStatus = &nnfv1alpha7.NnfDataMovementCommandStatus{}
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
		if ProgressCollectionEnabled(progressCollectInterval) {
			chCommandDone <- true // tell the process goroutine to stop parsing output
			<-chProgressDone      // wait for process goroutine to stop parsing final output
		}

		// Command is finished, update status
		now := metav1.NowMicro()
		dm.Status.EndTime = &now
		dm.Status.State = nnfv1alpha7.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonSuccess

		// Grab the output and trim it to remove the progress bloat
		output := TrimDcpProgressFromOutput(combinedOutBuf.String())

		// On cancellation or failure, log the output. On failure, also store the output in the
		// Status.Message. When successful, check the profile/UserConfig config options to log
		// and/or store the output.
		if errors.Is(ctxCancel.Err(), context.Canceled) {
			log.Info("Data movement operation cancelled", "output", output)
			dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonCancelled
		} else if err != nil {
			log.Error(err, "Data movement operation failed", "output", output)
			dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonFailed
			dm.Status.Message = fmt.Sprintf("%s: %s", err.Error(), output)
			resourceErr := dwsv1alpha3.NewResourceError("").WithError(err).WithUserMessage("data movement operation failed: %s", output).WithFatal()
			dm.Status.SetResourceErrorAndLog(resourceErr, log)
		} else {
			log.Info("Data movement operation completed", "cmdStatus", cmdStatus)

			// Profile or DM request has enabled stdout logging
			if profile.Data.LogStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.LogStdout) {
				log.Info("Data movement operation output", "output", output)
			}

			// Profile or DM request has enabled storing stdout
			if profile.Data.StoreStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.StoreStdout) {
				dm.Status.Message = output
			}
		}

		os.RemoveAll(filepath.Dir(mpiHostfile))

		status := dm.Status.DeepCopy()

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			dm := &nnfv1alpha7.NnfDataMovement{}
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

func (r *DataMovementReconciler) cancel(ctx context.Context, dm *nnfv1alpha7.NnfDataMovement) error {
	log := log.FromContext(ctx)

	// Check for the scenario where a request is canceled but not deleted before the DM has started.
	// If so, record it as cancelled and do nothing more with the data movement operation
	if dm.Status.StartTime.IsZero() && !dm.DeletionTimestamp.IsZero() {
		now := metav1.NowMicro()
		dm.Status.State = nnfv1alpha7.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonCancelled
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

	cancelContext := storedCancelContext.(DataMovementCancelContext)

	log.Info("Cancelling operation")
	cancelContext.Cancel()
	<-cancelContext.Ctx.Done()

	// Nothing more to do - the go routine that is executing the data movement will exit
	// and the status is recorded then.

	return nil
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
		For(&nnfv1alpha7.NnfDataMovement{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxReconciles}).
		WithEventFilter(filterByNamespace(r.WatchNamespace)).
		Complete(r)
}
