/*
 * Copyright 2024-2025 Hewlett Packard Enterprise Development LP
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

package driver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.openly.dev/pointy"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dwsv1alpha5 "github.com/DataWorkflowServices/dws/api/v1alpha5"
	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	"github.com/NearNodeFlash/nnf-dm/internal/controller/helpers"
	nnfv1alpha7 "github.com/NearNodeFlash/nnf-sos/api/v1alpha7"
)

var (
	// The name prefix used for generating NNF Data Movement operations.
	nameBase     = "nnf-copy-offload-"
	nodeNameBase = "nnf-copy-offload-node-"
)

// Driver will have only one instance per process, shared by all threads
// in the process.
type Driver struct {
	Client client.Client
	Log    logr.Logger

	Mock      bool
	MockCount int

	WorkflowName      string
	WorkflowNamespace string

	// We maintain a map of active operations which allows us to process cancel requests.
	// This is a thread safe map since multiple data movement reconcilers and go routines will be executing at the same time.
	// The SrvrDataMovementRecord objects are stored here.
	contexts sync.Map

	// A map of operation completions. When one is completed, its name is recorded
	// here until it can be reaped.
	completions map[string]struct{}
	cond        *sync.Cond
}

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress.
// These objects are stored in the Driver.contexts map.
type SrvrDataMovementRecord struct {
	cancelContext helpers.DataMovementCancelContext

	// Save a reference to the NnfDataMovement when in mock mode.
	mockDM *nnfv1alpha7.NnfDataMovement
}

// DriverRequest contains the information that is collected during the initial examination
// of a given http request. This has a one-to-one relationship with a DMRequest
// object.
type DriverRequest struct {
	Drvr *Driver

	// Fresh copy of the chosen NnfDataMovementProfile.
	dmProfile *nnfv1alpha7.NnfDataMovementProfile
	// Storage nodes to use.
	nodes []string
	// Worker nodes to use.
	hosts []string
	// MPI hosts file.
	mpiHostfile string
	// Rabbit node targeted for data movement. This is the local node attach to the requesting
	// compute node.
	RabbitName string
	// Command arguments.
	cmdArgs []string
}

func NewDriver(log logr.Logger, mock bool) (*Driver, error) {
	workflowName := os.Getenv("DW_WORKFLOW_NAME")
	workflowNamespace := os.Getenv("DW_WORKFLOW_NAMESPACE")
	if workflowName == "" || workflowNamespace == "" {
		return nil, errors.New("DW_WORKFLOW_NAME and DW_WORKFLOW_NAMESPACE must be set")
	}
	return &Driver{
		Log:               log,
		Mock:              mock,
		WorkflowName:      workflowName,
		WorkflowNamespace: workflowNamespace,
		completions:       make(map[string]struct{}),
		cond:              sync.NewCond(&sync.Mutex{}),
	}, nil
}

func (r *DriverRequest) Create(ctx context.Context, dmreq DMRequest) (string, *nnfv1alpha7.NnfDataMovement, error) {

	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", drvr.WorkflowName)
	workflow := &dwsv1alpha5.Workflow{}

	wf := types.NamespacedName{Name: drvr.WorkflowName, Namespace: drvr.WorkflowNamespace}
	if err := drvr.Client.Get(ctx, wf, workflow); err != nil {
		crLog.Info("Unable to get workflow: %v", err)
		return "", nil, err
	}
	if workflow.Status.State != dwsv1alpha5.StatePreRun || workflow.Status.Status != "Completed" {
		err := fmt.Errorf("workflow must be in '%s' state and 'Completed' status", dwsv1alpha5.StatePreRun)
		crLog.Error(err, "Workflow is in an invalid state")
		return "", nil, err
	}

	computeClientMount, computeMountInfo, err := r.findComputeMountInfo(ctx, dmreq)
	if err != nil {
		crLog.Error(err, "Failed to retrieve compute mountinfo")
		return "", nil, err
	}

	// Determine which rabbit is local to the compute that made the request
	rabbit, err := r.findRabbitNameFromCompute(dmreq.ComputeName)
	if err != nil {
		crLog.Error(err, "Failed to trace compute node to its rabbit node")
		return "", nil, err
	}
	r.RabbitName = rabbit

	crLog = crLog.WithValues("type", computeMountInfo.Type)
	var dm *nnfv1alpha7.NnfDataMovement
	switch computeMountInfo.Type {
	case "lustre":
		dm, err = r.createNnfDataMovement(ctx, dmreq, computeMountInfo, computeClientMount)
	case "gfs2":
		dm, err = r.createNnfNodeDataMovement(ctx, dmreq, computeMountInfo)
	default:
		// xfs is not supported since it can only be mounted in one location at a time. It is
		// already mounted on the compute node when copy offload occurs (PreRun/PostRun), therefore
		// it cannot be mounted on the rabbit to perform data movement.
		err = fmt.Errorf("filesystem not supported")
	}

	if err != nil {
		crLog.Error(err, "Failed to create DM request")
		return "", nil, err
	}

	// Dm Profile - no pinned profiles here since copy_offload could use any profile.
	r.dmProfile, err = r.selectProfile(ctx, dmreq)
	if err != nil {
		crLog.Error(err, "Failed to get data movement profile", "wanted", dmreq.DMProfile)
		return "", nil, err
	}
	dm.Spec.ProfileReference = corev1.ObjectReference{
		Kind:      reflect.TypeOf(nnfv1alpha7.NnfDataMovementProfile{}).Name(),
		Name:      r.dmProfile.Name,
		Namespace: r.dmProfile.Namespace,
	}
	crLog.Info("Using NnfDataMovmentProfile", "name", r.dmProfile.Name)

	dm.Spec.UserId = workflow.Spec.UserID
	dm.Spec.GroupId = workflow.Spec.GroupID

	// Add appropriate workflow labels so this is cleaned up
	dwsv1alpha5.AddWorkflowLabels(dm, workflow)
	dwsv1alpha5.AddOwnerLabels(dm, workflow)

	// Label the NnfDataMovement with a teardown state of "post_run" so the NNF workflow
	// controller can identify compute initiated data movements.
	nnfv1alpha7.AddDataMovementTeardownStateLabel(dm, dwsv1alpha5.StatePostRun)

	// Allow the user to override/supplement certain settings
	setUserConfig(dmreq, dm)

	r.nodes, err = helpers.GetStorageNodeNames(drvr.Client, ctx, dm)
	if err != nil {
		crLog.Error(err, "could not get storage nodes for data movement")
		return "", nil, err
	}

	// Get the FQDNs of the worker nodes so we can use them to create the mpirun hostfile
	r.hosts, err = helpers.GetCopyOffloadWorkerHostnames(drvr.Client, ctx, r.nodes, drvr.WorkflowName, drvr.WorkflowNamespace, dm)
	if err != nil {
		crLog.Error(err, "could not get worker nodes for data movement")
		return "", nil, err
	}

	// Create the hostfile used by `mpirun`. This is needed for preparing the destination
	// and the data movement command itself.
	r.mpiHostfile, err = helpers.CreateMpiHostfile(r.dmProfile, r.hosts, dm)
	if err != nil {
		crLog.Error(err, "could not create MPI hostfile")
		return "", nil, err
	}
	crLog.Info("MPI Hostfile preview", "first line", helpers.PeekMpiHostfile(r.mpiHostfile))

	// Prepare Destination Directory
	if err := helpers.PrepareDestination(drvr.Client, ctx, r.dmProfile, dm, r.mpiHostfile, crLog); err != nil {
		crLog.Error(err, "could not prepare destination")
		return "", nil, err
	}

	// Build command
	r.cmdArgs, err = helpers.BuildDMCommand(r.dmProfile, r.mpiHostfile, false, dm, crLog)
	if err != nil {
		crLog.Error(err, "could not create data movement command")
		return "", nil, err
	}

	if err := drvr.Client.Create(ctx, dm); err != nil {
		crLog.Error(err, "Failed to create NnfDataMovement")
		return "", nil, err
	}
	crLog.Info("Created NnfDataMovement", "name", dm.Name)
	r.recordRequest(ctx, dm)

	return r.dmKey(dm), dm, nil
}

func (r *DriverRequest) CreateMock(ctx context.Context, dmreq DMRequest) (string, *nnfv1alpha7.NnfDataMovement, error) {
	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", drvr.WorkflowName)
	var err error

	r.RabbitName = "mock-rabbit-01"
	dm := &nnfv1alpha7.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nodeNameBase,
			Namespace:    r.RabbitName, // Use the rabbit
			Labels: map[string]string{
				nnfv1alpha7.DataMovementInitiatorLabel: dmreq.ComputeName,
			},
		},
		Spec: nnfv1alpha7.NnfDataMovementSpec{
			Source: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path: dmreq.SourcePath,
			},
			Destination: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path: dmreq.DestinationPath,
			},
		},
	}

	r.dmProfile = &nnfv1alpha7.NnfDataMovementProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock",
			Namespace: nnfv1alpha7.DataMovementNamespace,
		},
		Data: nnfv1alpha7.NnfDataMovementProfileData{
			ProgressIntervalSeconds: 1,
			Command:                 "sleep 300",
		},
	}

	// Allow the user to override/supplement certain settings
	setUserConfig(dmreq, dm)

	// We name the NnfDataMovement ourselves, since we're not giving it to k8s.
	// We'll use this name internally.
	nameSuffix := fmt.Sprintf("%d", drvr.MockCount)
	drvr.MockCount += 1
	dm.Name = fmt.Sprintf("%s%s", dm.GetObjectMeta().GetGenerateName(), nameSuffix)

	// Build command
	r.cmdArgs, err = helpers.BuildDMCommand(r.dmProfile, r.mpiHostfile, false, dm, crLog)
	if err != nil {
		crLog.Error(err, "could not create data movement command")
		return "", nil, err
	}

	r.recordRequest(ctx, dm)
	return r.dmKey(dm), dm, nil
}

func (r *DriverRequest) Drive(ctx context.Context, dmreq DMRequest, dm *nnfv1alpha7.NnfDataMovement) error {
	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", drvr.WorkflowName)

	contextRecord, err := r.loadRequest(dm)
	if err != nil {
		crLog.Error(err, "request not found")
		return err
	}
	cancelContext := contextRecord.cancelContext
	if err := r.driveWithContext(ctx, cancelContext.Ctx, dm, crLog); err != nil {
		crLog.Error(err, "failed copy")
		os.RemoveAll(filepath.Dir(r.mpiHostfile))
		r.deleteRequest(dm)
		r.notifyCompletion(dm)
		return err
	}

	return nil
}

func (r *DriverRequest) DriveMock(ctx context.Context, dmreq DMRequest, dm *nnfv1alpha7.NnfDataMovement) error {
	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", drvr.WorkflowName)

	contextRecord, err := r.loadRequest(dm)
	if err != nil {
		crLog.Error(err, "request not found")
		return err
	}
	cancelContext := contextRecord.cancelContext
	if err := r.driveWithContext(ctx, cancelContext.Ctx, dm, crLog); err != nil {
		crLog.Error(err, "failed copy")
		r.deleteRequest(dm)
		r.notifyCompletion(dm)
		return err
	}

	return nil
}

func (r *DriverRequest) dmKey(dm *nnfv1alpha7.NnfDataMovement) string {
	return fmt.Sprintf("%s--%s", dm.Namespace, dm.Name)
}

func (r *DriverRequest) dmKeySplit(key string) (string, string) {
	split := strings.Split(key, "--")
	return split[0], split[1]
}

func (r *DriverRequest) recordRequest(ctx context.Context, dm *nnfv1alpha7.NnfDataMovement) {
	drvr := r.Drvr

	// Expand the context with cancel and store it in the map so the cancel function can be
	// found by another server thread if necessary.
	ctxCancel, cancel := context.WithCancel(ctx)
	record := SrvrDataMovementRecord{
		cancelContext: helpers.DataMovementCancelContext{
			Ctx:    ctxCancel,
			Cancel: cancel,
		},
	}
	if drvr.Mock {
		record.mockDM = dm
	}
	drvr.contexts.Store(r.dmKey(dm), record)
}

func (r *DriverRequest) loadRequest(dm *nnfv1alpha7.NnfDataMovement) (SrvrDataMovementRecord, error) {
	return r.loadRequestByName(r.dmKey(dm))
}

func (r *DriverRequest) loadRequestByName(name string) (SrvrDataMovementRecord, error) {
	storedCancelContext, loaded := r.Drvr.contexts.Load(name)
	if !loaded {
		return SrvrDataMovementRecord{}, fmt.Errorf("request not found")
	}
	return storedCancelContext.(SrvrDataMovementRecord), nil
}

func (r *DriverRequest) deleteRequest(dm *nnfv1alpha7.NnfDataMovement) {
	r.Drvr.contexts.Delete(r.dmKey(dm))
}

func (r *DriverRequest) CancelRequest(ctx context.Context, name string) error {
	drvr := r.Drvr

	contextRecord, err := r.loadRequestByName(name)
	if err != nil {
		// Maybe the Go routine already finished and removed the record.
		return err
	}
	cancelContext := contextRecord.cancelContext
	drvr.Log.Info("cancelling request", "name", name)
	cancelContext.Cancel()
	<-cancelContext.Ctx.Done()

	// Nothing more to do - the go routine that is executing the data movement will exit
	// and the status is recorded then.

	return nil
}

func (r *DriverRequest) GetRequestMock(ctx context.Context, statreq StatusRequest) (*DataMovementStatusResponse_v1_0, int, error) {
	contextRecord, err := r.loadRequestByName(statreq.RequestName)
	if err != nil {
		return nil, http.StatusNotFound, errors.New("request not found")
	}
	dm := contextRecord.mockDM

	statusResponse, code, err := r.buildStatusResponse(statreq, dm)
	// A static timestamp that the tests know.
	statusResponse.StartTime = "2025-04-01 11:46:36.535519 -0500 CDT"
	return statusResponse, code, err
}

func (r *DriverRequest) GetRequest(ctx context.Context, statreq StatusRequest) (*DataMovementStatusResponse_v1_0, int, error) {
	drvr := r.Drvr
	keyNS, keyName := r.dmKeySplit(statreq.RequestName)
	dmReq := types.NamespacedName{Name: keyName, Namespace: keyNS}
	dm := &nnfv1alpha7.NnfDataMovement{}
	crLog := drvr.Log.WithValues("workflow", drvr.WorkflowName, "request", statreq.RequestName)

	drvr.Log.Info("Getting request", "request", dmReq)
	if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
		if apierrors.IsNotFound(err) {
			r.deleteCompletionByName(statreq.RequestName)
			return nil, http.StatusNotFound, errors.New("request not found")
		}
		crLog.Error(err, "failed to get NnfDataMovement resource")
		return nil, http.StatusInternalServerError, err
	}
	if dm.Labels[dwsv1alpha5.WorkflowNameLabel] != drvr.WorkflowName || dm.Labels[dwsv1alpha5.WorkflowNamespaceLabel] != drvr.WorkflowNamespace {
		return nil, http.StatusBadRequest, errors.New("request does not belong to workflow")
	}

	if dm.Status.EndTime.IsZero() && statreq.MaxWaitSecs != 0 {

		r.waitForCompletionOrTimeout(statreq)

		if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
			if apierrors.IsNotFound(err) {
				r.deleteCompletionByName(statreq.RequestName)
				return nil, http.StatusNotFound, errors.New("request not found after waiting for completion")
			}
			crLog.Error(err, "failed to get NnfDataMovement resource afer waiting for completion")
			return nil, http.StatusInternalServerError, err
		}
	}

	statusResponse, code, err := r.buildStatusResponse(statreq, dm)
	return statusResponse, code, err
}

func (r *DriverRequest) buildStatusResponse(statreq StatusRequest, dm *nnfv1alpha7.NnfDataMovement) (*DataMovementStatusResponse_v1_0, int, error) {

	if dm.Status.StartTime.IsZero() && dm.Status.Status != nnfv1alpha7.DataMovementConditionReasonInvalid {
		return &DataMovementStatusResponse_v1_0{
			State:  DataMovementStatusResponse_PENDING,
			Status: DataMovementStatusResponse_UNKNOWN_STATUS,
		}, -1, nil
	}

	stateMap := map[string]DataMovementStatusResponse_State{
		"": DataMovementStatusResponse_UNKNOWN_STATE,
		nnfv1alpha7.DataMovementConditionTypeStarting: DataMovementStatusResponse_STARTING,
		nnfv1alpha7.DataMovementConditionTypeRunning:  DataMovementStatusResponse_RUNNING,
		nnfv1alpha7.DataMovementConditionTypeFinished: DataMovementStatusResponse_COMPLETED,
	}

	state, ok := stateMap[dm.Status.State]
	if !ok {
		return nil, http.StatusInternalServerError, errors.New("failed to decode returned state")
	}

	if state != DataMovementStatusResponse_COMPLETED && dm.Spec.Cancel {
		state = DataMovementStatusResponse_CANCELLING
	}

	statusMap := map[string]DataMovementStatusResponse_Status{
		"": DataMovementStatusResponse_UNKNOWN_STATUS,
		nnfv1alpha7.DataMovementConditionReasonFailed:    DataMovementStatusResponse_FAILED,
		nnfv1alpha7.DataMovementConditionReasonSuccess:   DataMovementStatusResponse_SUCCESS,
		nnfv1alpha7.DataMovementConditionReasonInvalid:   DataMovementStatusResponse_INVALID,
		nnfv1alpha7.DataMovementConditionReasonCancelled: DataMovementStatusResponse_CANCELLED,
	}

	status, ok := statusMap[dm.Status.Status]
	if !ok {
		return nil, http.StatusInternalServerError, errors.New("failed to decode returned status")
	}

	cmdStatus := DataMovementCommandStatus{}
	if dm.Status.CommandStatus != nil {
		cmdStatus.Command = dm.Status.CommandStatus.Command
		cmdStatus.LastMessage = dm.Status.CommandStatus.LastMessage

		if dm.Status.CommandStatus.ElapsedTime.Duration > 0 {
			d := dm.Status.CommandStatus.ElapsedTime.Truncate(time.Millisecond)
			cmdStatus.ElapsedTime = d.String()
		}
		if !dm.Status.CommandStatus.LastMessageTime.IsZero() {
			cmdStatus.LastMessageTime = dm.Status.CommandStatus.LastMessageTime.Local().String()
		}
		if dm.Status.CommandStatus.ProgressPercentage != nil {
			cmdStatus.Progress = *dm.Status.CommandStatus.ProgressPercentage
		}
	}

	startTimeStr := ""
	if !dm.Status.StartTime.IsZero() {
		startTimeStr = dm.Status.StartTime.Local().String()
	}
	endTimeStr := ""
	if !dm.Status.EndTime.IsZero() {
		endTimeStr = dm.Status.EndTime.Local().String()
	}

	if state == DataMovementStatusResponse_COMPLETED && !dm.Status.EndTime.IsZero() {
		r.deleteCompletionByName(statreq.RequestName)
	}

	return &DataMovementStatusResponse_v1_0{
		State:         state,
		Status:        status,
		Message:       dm.Status.Message,
		CommandStatus: &cmdStatus,
		StartTime:     startTimeStr,
		EndTime:       endTimeStr,
	}, -1, nil
}

func (r *DriverRequest) ListRequests(ctx context.Context) ([]string, error) {
	drvr := r.Drvr

	drvr.Log.Info("Listing requests:")
	items := make([]string, 0)
	drvr.contexts.Range(func(key, val interface{}) bool {
		skey := fmt.Sprintf("%s", key)
		drvr.Log.Info(fmt.Sprintf("   %s", skey))
		items = append(items, skey)
		return true
	})
	slices.Sort(items)

	return items, nil
}

func (r *DriverRequest) driveWithContext(ctx context.Context, ctxCancel context.Context, dm *nnfv1alpha7.NnfDataMovement, crLog logr.Logger) error {
	drvr := r.Drvr
	dmReq := types.NamespacedName{Name: dm.Name, Namespace: dm.Namespace}

	cmd := exec.CommandContext(ctxCancel, "/bin/bash", "-c", strings.Join(r.cmdArgs, " "))
	cmdStatus := nnfv1alpha7.NnfDataMovementCommandStatus{}
	cmdStatus.Command = cmd.String()

	setStart := func() {
		now := metav1.NowMicro()
		dm.Status.StartTime = &now
		dm.Status.State = nnfv1alpha7.DataMovementConditionTypeRunning
		dm.Status.CommandStatus = &cmdStatus
	}

	// Record the start of the data movement operation
	if !drvr.Mock {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
				return client.IgnoreNotFound(err)
			}
			setStart()
			return drvr.Client.Status().Update(ctx, dm)
		})
		if err != nil {
			crLog.Error(err, "failed to update CommandStatus with start time", "cmdStatus", cmdStatus)
			return err
		}
	} else {
		setStart()
	}

	crLog.Info("Running Command", "cmd", cmdStatus.Command)
	contextDelete := func() {
		r.deleteRequest(dm)
		r.notifyCompletion(dm)
	}
	r.runit(ctx, ctxCancel, contextDelete, dmReq, cmd, &cmdStatus, r.dmProfile, r.mpiHostfile, crLog)
	return nil
}

func (r *DriverRequest) runit(ctx context.Context, ctxCancel context.Context, contextDelete func(), dmReq types.NamespacedName, cmd *exec.Cmd, cmdStatus *nnfv1alpha7.NnfDataMovementCommandStatus, profile *nnfv1alpha7.NnfDataMovementProfile, mpiHostfile string, log logr.Logger) {
	drvr := r.Drvr

	// Execute the go routine to perform the data movement
	go func() {
		var err error
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
		if helpers.ProgressCollectionEnabled(progressCollectInterval) {
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
						if err := helpers.ParseDcpProgress(line, cmdStatus); err != nil {
							log.Error(err, "failed to parse progress", "line", line)
							return
						}

						// Collect stats only when finished
						if cmdStatus.ProgressPercentage != nil && *cmdStatus.ProgressPercentage >= 100 {
							if err := helpers.ParseDcpStats(line, cmdStatus); err != nil {
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
					if !drvr.Mock {
						err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
							dm := &nnfv1alpha7.NnfDataMovement{}
							if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
								return client.IgnoreNotFound(err)
							}

							if dm.Status.CommandStatus == nil {
								dm.Status.CommandStatus = &nnfv1alpha7.NnfDataMovementCommandStatus{}
							}
							cmdStatus.DeepCopyInto(dm.Status.CommandStatus)

							return drvr.Client.Status().Update(ctx, dm)
						})

						if err != nil {
							log.Error(err, "failed to update CommandStatus with Progress", "cmdStatus", cmdStatus)
						}
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

		err = cmd.Wait()

		// If enabled, wait for final progress collection
		if helpers.ProgressCollectionEnabled(progressCollectInterval) {
			chCommandDone <- true // tell the process goroutine to stop parsing output
			<-chProgressDone      // wait for process goroutine to stop parsing final output
		}

		// Command is finished, update status
		dm := &nnfv1alpha7.NnfDataMovement{}
		if !drvr.Mock {
			if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
				log.Error(err, "failed to get NnfDataMovement resource for final status update")
				return
			}
		}
		now := metav1.NowMicro()
		dm.Status.EndTime = &now
		dm.Status.State = nnfv1alpha7.DataMovementConditionTypeFinished
		dm.Status.Status = nnfv1alpha7.DataMovementConditionReasonSuccess

		// Grab the output and trim it to remove the progress bloat
		output := helpers.TrimDcpProgressFromOutput(combinedOutBuf.String())

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
			resourceErr := dwsv1alpha5.NewResourceError("").WithError(err).WithUserMessage("data movement operation failed: %s", output).WithFatal()
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

		if !drvr.Mock {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				dm := &nnfv1alpha7.NnfDataMovement{}
				if err := drvr.Client.Get(ctx, dmReq, dm); err != nil {
					return client.IgnoreNotFound(err)
				}

				// Ensure we have the latest CommandStatus from the progress goroutine
				cmdStatus.DeepCopyInto(status.CommandStatus)
				status.DeepCopyInto(&dm.Status)

				return drvr.Client.Status().Update(ctx, dm)
			})

			if err != nil {
				log.Error(err, "failed to update dm status with completion")
				// TODO Add prometheus counter to track occurrences
			}
		}

		contextDelete()
	}()
}

func (r *DriverRequest) notifyCompletion(dm *nnfv1alpha7.NnfDataMovement) {
	drvr := r.Drvr
	key := r.dmKey(dm)

	drvr.cond.L.Lock()
	drvr.completions[key] = struct{}{}
	drvr.cond.L.Unlock()
	drvr.cond.Broadcast()
}

func (r *DriverRequest) deleteCompletionByName(name string) {
	drvr := r.Drvr

	drvr.cond.L.Lock()
	delete(drvr.completions, name)
	drvr.cond.L.Unlock()
}

// waitForCompletionOrTimeout waits for the completion of the status request or for timeout.
func (r *DriverRequest) waitForCompletionOrTimeout(statreq StatusRequest) {
	drvr := r.Drvr
	timeout := false
	complete := make(chan struct{}, 1)

	// Start a go routine that waits for the completion of this status request or for timeout
	go func() {

		drvr.cond.L.Lock()
		for {
			if _, found := drvr.completions[statreq.RequestName]; found || timeout {
				break
			}

			drvr.cond.Wait()
		}

		drvr.cond.L.Unlock()

		complete <- struct{}{}
	}()

	var maxWaitTimeDuration time.Duration
	if statreq.MaxWaitSecs < 0 || time.Duration(statreq.MaxWaitSecs) > math.MaxInt64/time.Second {
		maxWaitTimeDuration = math.MaxInt64
	} else {
		maxWaitTimeDuration = time.Duration(statreq.MaxWaitSecs) * time.Second
	}

	// Wait for completion signal or timeout
	select {
	case <-complete:
	case <-time.After(maxWaitTimeDuration):
		timeout = true
		drvr.cond.Broadcast() // Wake up everyone (including myself) to close out the running go routine
	}
}

// Set the DM's UserConfig options based on the incoming requests's options
func setUserConfig(dmreq DMRequest, dm *nnfv1alpha7.NnfDataMovement) {
	dm.Spec.UserConfig = &nnfv1alpha7.NnfDataMovementConfig{}
	dm.Spec.UserConfig.Dryrun = dmreq.Dryrun
	dm.Spec.UserConfig.MpirunOptions = dmreq.MpirunOptions
	dm.Spec.UserConfig.DcpOptions = dmreq.DcpOptions
	dm.Spec.UserConfig.LogStdout = dmreq.LogStdout
	dm.Spec.UserConfig.StoreStdout = dmreq.StoreStdout

	// TODO: Even though these aren't set explicity by copy offload API, they are getting
	// overwritten. Investigate this. The profile says 8, but we only get 1 slot. I did remove
	// slotsPerWorker from the container profile, so perhaps mpi-operator is doing someting extra
	// there too.
	if dmreq.Slots >= 0 {
		dm.Spec.UserConfig.Slots = pointy.Int(int(dmreq.Slots))
	}
	if dmreq.MaxSlots >= 0 {
		dm.Spec.UserConfig.MaxSlots = pointy.Int(int(dmreq.MaxSlots))
	}
}

// createNnfNodeDataMovement creates an NnfDataMovement to be used with Lustre.
func (r *DriverRequest) createNnfDataMovement(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha5.ClientMountInfo, computeClientMount *dwsv1alpha5.ClientMount) (*nnfv1alpha7.NnfDataMovement, error) {

	// Find the ClientMount for the rabbit.
	source, err := r.findRabbitRelativeSource(ctx, dmreq, computeMountInfo)
	if err != nil {
		return nil, err
	}

	var dwIndex string
	if dw, err := getDirectiveIndexFromClientMount(computeClientMount); err != nil {
		return nil, err
	} else {
		dwIndex = dw
	}

	lustrefs, err := r.findDestinationLustreFilesystem(ctx, dmreq.DestinationPath)
	if err != nil {
		return nil, err
	}

	dm := &nnfv1alpha7.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			// Be careful about how much you put into GenerateName.
			// The MPI operator will use the resulting name as a
			// prefix for its own names.
			GenerateName: nameBase,
			// Use the data movement namespace.
			Namespace: nnfv1alpha7.DataMovementNamespace,
			Labels: map[string]string{
				nnfv1alpha7.DataMovementInitiatorLabel: dmreq.ComputeName,
				nnfv1alpha7.DirectiveIndexLabel:        dwIndex,
			},
		},
		Spec: nnfv1alpha7.NnfDataMovementSpec{
			Source: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path:             source,
				StorageReference: computeMountInfo.Device.DeviceReference.ObjectReference,
			},
			Destination: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path: dmreq.DestinationPath,
				StorageReference: corev1.ObjectReference{
					Kind:      reflect.TypeOf(*lustrefs).Name(),
					Namespace: lustrefs.Namespace,
					Name:      lustrefs.Name,
				},
			},
		},
	}

	return dm, nil
}

func getDirectiveIndexFromClientMount(object *dwsv1alpha5.ClientMount) (string, error) {
	// Find the DW index for our work.
	labels := object.GetLabels()
	if labels == nil {
		return "", fmt.Errorf("unable to find labels on compute ClientMount, namespaces=%s, name=%s", object.Namespace, object.Name)
	}

	dwIndex, found := labels[nnfv1alpha7.DirectiveIndexLabel]
	if !found {
		return "", fmt.Errorf("unable to find directive index label on compute ClientMount, namespace=%s name=%s", object.Namespace, object.Name)
	}

	return dwIndex, nil
}

// createNnfNodeDataMovement creates an NnfDataMovement to be used with GFS2.
func (r *DriverRequest) createNnfNodeDataMovement(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha5.ClientMountInfo) (*nnfv1alpha7.NnfDataMovement, error) {

	// Find the ClientMount for the rabbit.
	source, err := r.findRabbitRelativeSource(ctx, dmreq, computeMountInfo)
	if err != nil {
		return nil, err
	}

	dm := &nnfv1alpha7.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nodeNameBase,
			Namespace:    r.RabbitName, // Use the rabbit
			Labels: map[string]string{
				nnfv1alpha7.DataMovementInitiatorLabel: dmreq.ComputeName,
			},
		},
		Spec: nnfv1alpha7.NnfDataMovementSpec{
			Source: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path:             source,
				StorageReference: computeMountInfo.Device.DeviceReference.ObjectReference,
			},
			Destination: &nnfv1alpha7.NnfDataMovementSpecSourceDestination{
				Path: dmreq.DestinationPath,
			},
		},
	}

	return dm, nil
}

func (r *DriverRequest) findRabbitRelativeSource(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha5.ClientMountInfo) (string, error) {

	drvr := r.Drvr
	// Now look up the client mount on this Rabbit node and find the compute initiator. We append the relative path
	// to this value resulting in the full path on the Rabbit.

	listOptions := []client.ListOption{
		client.InNamespace(r.RabbitName),
		client.MatchingLabels(map[string]string{
			dwsv1alpha5.WorkflowNameLabel:      drvr.WorkflowName,
			dwsv1alpha5.WorkflowNamespaceLabel: drvr.WorkflowNamespace,
		}),
	}

	clientMounts := &dwsv1alpha5.ClientMountList{}
	if err := drvr.Client.List(ctx, clientMounts, listOptions...); err != nil {
		return "", err
	}

	if len(clientMounts.Items) == 0 {
		return "", fmt.Errorf("no client mounts found for node '%s'", r.RabbitName)
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if *computeMountInfo.Device.DeviceReference == *mount.Device.DeviceReference {
				return mount.MountPath + strings.TrimPrefix(dmreq.SourcePath, computeMountInfo.MountPath), nil
			}
		}
	}

	return "", fmt.Errorf("initiator not found in list of client mounts: %s", dmreq.ComputeName)
}

// Look up the client mounts on this node to find the compute relative mount path. The "spec.Source" must be
// prefixed with a mount path in the list of mounts. Once we find this mount, we can strip out the prefix and
// are left with the relative path.
func (r *DriverRequest) findComputeMountInfo(ctx context.Context, dmreq DMRequest) (*dwsv1alpha5.ClientMount, *dwsv1alpha5.ClientMountInfo, error) {

	drvr := r.Drvr
	listOptions := []client.ListOption{
		client.InNamespace(dmreq.ComputeName),
		client.MatchingLabels(map[string]string{
			dwsv1alpha5.WorkflowNameLabel:      drvr.WorkflowName,
			dwsv1alpha5.WorkflowNamespaceLabel: drvr.WorkflowNamespace,
		}),
	}

	clientMounts := &dwsv1alpha5.ClientMountList{}
	if err := drvr.Client.List(ctx, clientMounts, listOptions...); err != nil {
		return nil, nil, err
	}

	if len(clientMounts.Items) == 0 {
		return nil, nil, fmt.Errorf("no client mounts found for node '%s'", dmreq.ComputeName)
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if strings.HasPrefix(dmreq.SourcePath, mount.MountPath) {
				if mount.Device.DeviceReference == nil && mount.Device.Type != "lustre" {
					return nil, nil, fmt.Errorf("ClientMount %s/%s: Source path '%s' does not have device reference", clientMount.Namespace, clientMount.Name, dmreq.SourcePath)
				}

				return &clientMount, &mount, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("source path not found in list of client mounts: %s", dmreq.SourcePath)
}

func (r *DriverRequest) findDestinationLustreFilesystem(ctx context.Context, dest string) (*lusv1beta1.LustreFileSystem, error) {

	drvr := r.Drvr
	if !filepath.IsAbs(dest) {
		return nil, fmt.Errorf("destination must be an absolute path")
	}
	origDest := dest
	if !strings.HasSuffix(dest, "/") {
		dest += "/"
	}

	lustrefsList := &lusv1beta1.LustreFileSystemList{}
	if err := drvr.Client.List(ctx, lustrefsList); err != nil {
		return nil, fmt.Errorf("unable to list LustreFileSystem resources: %s", err.Error())
	}
	if len(lustrefsList.Items) == 0 {
		return nil, fmt.Errorf("no LustreFileSystem resources found")
	}

	for _, lustrefs := range lustrefsList.Items {
		mroot := lustrefs.Spec.MountRoot
		if !strings.HasSuffix(mroot, "/") {
			mroot += "/"
		}
		if strings.HasPrefix(dest, mroot) {
			return &lustrefs, nil
		}
	}

	return nil, fmt.Errorf("unable to find a LustreFileSystem resource matching %s", origDest)
}

func (r *DriverRequest) selectProfile(ctx context.Context, dmreq DMRequest) (*nnfv1alpha7.NnfDataMovementProfile, error) {
	drvr := r.Drvr
	profileName := dmreq.DMProfile
	ns := "nnf-system"

	// If a profile is named then verify that it exists.  Otherwise, verify that a default profile
	// can be found.
	if len(profileName) == 0 {
		NnfDataMovementProfiles := &nnfv1alpha7.NnfDataMovementProfileList{}
		if err := drvr.Client.List(ctx, NnfDataMovementProfiles, &client.ListOptions{Namespace: ns}); err != nil {
			return nil, err
		}
		profilesFound := make([]string, 0, len(NnfDataMovementProfiles.Items))
		for _, profile := range NnfDataMovementProfiles.Items {
			if profile.Data.Default {
				objkey := client.ObjectKeyFromObject(&profile)
				profilesFound = append(profilesFound, objkey.Name)
			}
		}
		// Require that there be one and only one default.
		if len(profilesFound) == 0 {
			return nil, fmt.Errorf("unable to find a default NnfDataMovementProfile to use")
		} else if len(profilesFound) > 1 {
			return nil, fmt.Errorf("more than one default NnfDataMovementProfile found; unable to pick one: %v", profilesFound)
		}
		profileName = profilesFound[0]
	}

	profile := &nnfv1alpha7.NnfDataMovementProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: ns,
		},
	}

	err := drvr.Client.Get(ctx, client.ObjectKeyFromObject(profile), profile)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve NnfDataMovementProfile: %s", profileName)
	}

	return profile, nil
}

// Use the systemconfiguration to find the compute node's local rabbit node
func (r *DriverRequest) findRabbitNameFromCompute(compute string) (string, error) {
	drvr := r.Drvr
	systemConfigName := "default"

	systemConfig := &dwsv1alpha5.SystemConfiguration{}
	if err := drvr.Client.Get(context.TODO(), types.NamespacedName{Name: systemConfigName, Namespace: corev1.NamespaceDefault}, systemConfig); err != nil {
		return "", fmt.Errorf("failed to retrieve system configuration: %w", err)
	}

	for _, storageNode := range systemConfig.Spec.StorageNodes {
		for _, computeNode := range storageNode.ComputesAccess {
			if computeNode.Name == compute {
				return storageNode.Name, nil
			}
		}
	}

	return "", nil
}
