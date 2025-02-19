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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"go.openly.dev/pointy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	"github.com/NearNodeFlash/nnf-dm/internal/controller/helpers"
	nnfv1alpha6 "github.com/NearNodeFlash/nnf-sos/api/v1alpha6"
)

var (
	// The name prefix used for generating NNF Data Movement operations.
	nameBase     = "nnf-copy-offload-"
	nodeNameBase = "nnf-copy-offload-node-"
)

// Driver will have only one instance per process, shared by all threads
// in the process.
type Driver struct {
	Client     client.Client
	Log        logr.Logger
	RabbitName string

	Mock      bool
	MockCount int

	// We maintain a map of active operations which allows us to process cancel requests.
	// This is a thread safe map since multiple data movement reconcilers and go routines will be executing at the same time.
	// The SrvrDataMovementRecord objects are stored here.
	contexts sync.Map
}

// Keep track of the context and its cancel function so that we can track
// and cancel data movement operations in progress.
// These objects are stored in the Driver.contexts map.
type SrvrDataMovementRecord struct {
	dmreq         *nnfv1alpha6.NnfDataMovement
	cancelContext helpers.DataMovementCancelContext
}

// DriverRequest contains the information that is collected during the initial examination
// of a given http request. This has a one-to-one relationship with a DMRequest
// object.
type DriverRequest struct {
	Drvr *Driver

	// Fresh copy of the chosen NnfDataMovementProfile.
	dmProfile *nnfv1alpha6.NnfDataMovementProfile
	// Storage nodes to use.
	nodes []string
	// Worker nodes to use.
	hosts []string
	// MPI hosts file.
	mpiHostfile string
}

func (r *DriverRequest) Create(ctx context.Context, dmreq DMRequest) (*nnfv1alpha6.NnfDataMovement, error) {

	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", dmreq.WorkflowName)
	workflow := &dwsv1alpha2.Workflow{}

	wf := types.NamespacedName{Name: dmreq.WorkflowName, Namespace: dmreq.WorkflowNamespace}
	if err := drvr.Client.Get(ctx, wf, workflow); err != nil {
		crLog.Info("Unable to get workflow: %v", err)
		return nil, err
	}
	if workflow.Status.State != dwsv1alpha2.StatePreRun || workflow.Status.Status != "Completed" {
		err := fmt.Errorf("workflow must be in '%s' state and 'Completed' status", dwsv1alpha2.StatePreRun)
		crLog.Info("Workflow is in an invalid state: %v", err)
		return nil, err
	}

	computeClientMount, computeMountInfo, err := r.findComputeMountInfo(ctx, dmreq)
	if err != nil {
		crLog.Error(err, "Failed to retrieve compute mountinfo")
		return nil, err
	}

	crLog = crLog.WithValues("type", computeMountInfo.Type)
	var dm *nnfv1alpha6.NnfDataMovement
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
		crLog.Error(err, "Failed to copy files")
		return nil, err
	}

	// Dm Profile - no pinned profiles here since copy_offload could use any profile.
	r.dmProfile, err = r.selectProfile(ctx, dmreq)
	if err != nil {
		crLog.Error(err, "Failed to get profile", "profile", dmreq.DMProfile)
		return nil, err
	}
	dm.Spec.ProfileReference = corev1.ObjectReference{
		Kind:      reflect.TypeOf(nnfv1alpha6.NnfDataMovementProfile{}).Name(),
		Name:      r.dmProfile.Name,
		Namespace: r.dmProfile.Namespace,
	}
	crLog.Info("Using NnfDataMovmentProfile", "name", r.dmProfile)

	dm.Spec.UserId = workflow.Spec.UserID
	dm.Spec.GroupId = workflow.Spec.GroupID

	// Add appropriate workflow labels so this is cleaned up
	dwsv1alpha2.AddWorkflowLabels(dm, workflow)
	dwsv1alpha2.AddOwnerLabels(dm, workflow)

	// Label the NnfDataMovement with a teardown state of "post_run" so the NNF workflow
	// controller can identify compute initiated data movements.
	nnfv1alpha6.AddDataMovementTeardownStateLabel(dm, dwsv1alpha2.StatePostRun)

	// Allow the user to override/supplement certain settings
	setUserConfig(dmreq, dm)

	// We name the NnfDataMovement ourselves, since we're not giving it to k8s.
	// We'll use this name internally.
	r.generateName(dm)

	return dm, nil
}

func (r *DriverRequest) generateName(dm *nnfv1alpha6.NnfDataMovement) {
	drvr := r.Drvr

	var nameSuffix string
	if drvr.Mock {
		nameSuffix = fmt.Sprintf("%d", drvr.MockCount)
		drvr.MockCount += 1
	} else {
		nameSuffix = string(uuid.NewString()[0:10])
	}
	dm.Name = fmt.Sprintf("%s%s", dm.GetObjectMeta().GetGenerateName(), nameSuffix)
}

func (r *DriverRequest) CreateMock(ctx context.Context, dmreq DMRequest) (*nnfv1alpha6.NnfDataMovement, error) {
	drvr := r.Drvr

	dm := &nnfv1alpha6.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nodeNameBase,
			Namespace:    drvr.RabbitName, // Use the rabbit
			Labels: map[string]string{
				nnfv1alpha6.DataMovementInitiatorLabel: dmreq.ComputeName,
			},
		},
	}
	r.generateName(dm)
	return dm, nil
}

func (r *DriverRequest) Drive(ctx context.Context, dmreq DMRequest, dm *nnfv1alpha6.NnfDataMovement) error {

	var err error
	drvr := r.Drvr
	crLog := drvr.Log.WithValues("workflow", dmreq.WorkflowName)

	r.nodes, err = helpers.GetStorageNodeNames(drvr.Client, ctx, dm)
	if err != nil {
		crLog.Error(err, "could not get storage nodes for data movement")
		return err
	}

	r.hosts, err = helpers.GetWorkerHostnames(drvr.Client, ctx, r.nodes)
	if err != nil {
		crLog.Error(err, "could not get worker nodes for data movement")
		return err
	}

	// Create the hostfile. This is needed for preparing the destination and the data movement
	// command itself.
	r.mpiHostfile, err = helpers.CreateMpiHostfile(r.dmProfile, r.hosts, dm)
	if err != nil {
		crLog.Error(err, "could not create MPI hostfile")
		return err
	}
	crLog.Info("MPI Hostfile preview", "first line", helpers.PeekMpiHostfile(r.mpiHostfile))

	ctxCancel := r.recordRequest(ctx, dm)

	if err := r.driveWithContext(ctx, ctxCancel, dm, crLog); err != nil {
		crLog.Error(err, "failed copy")
		os.RemoveAll(filepath.Dir(r.mpiHostfile))
		drvr.contexts.Delete(dm.Name)
		return err
	}

	return nil
}

func (r *DriverRequest) DriveMock(ctx context.Context, dmreq DMRequest, dm *nnfv1alpha6.NnfDataMovement) error {
	_ = r.recordRequest(ctx, dm)

	return nil
}

func (r *DriverRequest) recordRequest(ctx context.Context, dm *nnfv1alpha6.NnfDataMovement) context.Context {
	drvr := r.Drvr

	// Expand the context with cancel and store it in the map so the cancel function can be
	// found by another server thread if necessary.
	ctxCancel, cancel := context.WithCancel(ctx)
	drvr.contexts.Store(dm.Name, SrvrDataMovementRecord{
		dmreq: dm,
		cancelContext: helpers.DataMovementCancelContext{
			Ctx:    ctxCancel,
			Cancel: cancel,
		},
	})
	return ctxCancel
}

func (r *DriverRequest) CancelRequest(ctx context.Context, name string) error {
	drvr := r.Drvr

	storedCancelContext, loaded := drvr.contexts.LoadAndDelete(name)
	if !loaded {
		// Maybe it already completed and removed itself?
		return nil
	}

	contextRecord := storedCancelContext.(SrvrDataMovementRecord)
	cancelContext := contextRecord.cancelContext
	drvr.Log.Info("cancelling request", "name", name)
	cancelContext.Cancel()
	<-cancelContext.Ctx.Done()

	// Nothing more to do - the go routine that is executing the data movement will exit
	// and the status is recorded then.

	return nil
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

func (r *DriverRequest) driveWithContext(ctx context.Context, ctxCancel context.Context, dm *nnfv1alpha6.NnfDataMovement, crLog logr.Logger) error {
	drvr := r.Drvr

	// Prepare Destination Directory
	if err := helpers.PrepareDestination(drvr.Client, ctx, r.dmProfile, dm, r.mpiHostfile, crLog); err != nil {
		return err
	}

	// Build command
	cmdArgs, err := helpers.BuildDMCommand(r.dmProfile, r.mpiHostfile, dm, crLog)
	if err != nil {
		crLog.Error(err, "could not create data movement command")
		return err
	}
	cmd := exec.CommandContext(ctxCancel, "/bin/bash", "-c", strings.Join(cmdArgs, " "))

	// XXX DEAN DEAN at some point we need a lock for the dm.Status,
	// to coordinate with a 'cancel' message that comes into the server.

	// Record the start of the data movement operation
	now := metav1.NowMicro()
	dm.Status.StartTime = &now
	dm.Status.State = nnfv1alpha6.DataMovementConditionTypeRunning
	cmdStatus := nnfv1alpha6.NnfDataMovementCommandStatus{}
	cmdStatus.Command = cmd.String()
	dm.Status.CommandStatus = &cmdStatus
	crLog.Info("Running Command", "cmd", cmdStatus.Command)

	contextDelete := func() { drvr.contexts.Delete(dm.Name) }

	runit(ctxCancel, contextDelete, cmd, &cmdStatus, r.dmProfile, r.mpiHostfile, crLog)
	return nil
}

func runit(ctxCancel context.Context, contextDelete func(), cmd *exec.Cmd, cmdStatus *nnfv1alpha6.NnfDataMovementCommandStatus, profile *nnfv1alpha6.NnfDataMovementProfile, mpiHostfile string, log logr.Logger) {

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

					//// Update the CommandStatus in the DM resource after we parsed all the lines
					//err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					//	dm := &nnfv1alpha6.NnfDataMovement{}
					//	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
					//		return client.IgnoreNotFound(err)
					//	}
					//
					//	if dm.Status.CommandStatus == nil {
					//		dm.Status.CommandStatus = &nnfv1alpha6.NnfDataMovementCommandStatus{}
					//	}
					//	cmdStatus.DeepCopyInto(dm.Status.CommandStatus)
					//
					//	return r.Status().Update(ctx, dm)
					//})

					//if err != nil {
					//	log.Error(err, "failed to update CommandStatus with Progress", "cmdStatus", cmdStatus)
					//}
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
		//now := metav1.NowMicro()
		//dm.Status.EndTime = &now
		//dm.Status.State = nnfv1alpha6.DataMovementConditionTypeFinished
		//dm.Status.Status = nnfv1alpha6.DataMovementConditionReasonSuccess

		// Grab the output and trim it to remove the progress bloat
		output := helpers.TrimDcpProgressFromOutput(combinedOutBuf.String())

		// On cancellation or failure, log the output. On failure, also store the output in the
		// Status.Message. When successful, check the profile/UserConfig config options to log
		// and/or store the output.
		if errors.Is(ctxCancel.Err(), context.Canceled) {
			log.Info("Data movement operation cancelled", "output", output)
			//dm.Status.Status = nnfv1alpha6.DataMovementConditionReasonCancelled
		} else if err != nil {
			log.Error(err, "Data movement operation failed", "output", output)
			//dm.Status.Status = nnfv1alpha6.DataMovementConditionReasonFailed
			//dm.Status.Message = fmt.Sprintf("%s: %s", err.Error(), output)
			//resourceErr := dwsv1alpha2.NewResourceError("").WithError(err).WithUserMessage("data movement operation failed: %s", output).WithFatal()
			//dm.Status.SetResourceErrorAndLog(resourceErr, log)
		} else {
			log.Info("Data movement operation completed", "cmdStatus", cmdStatus)

			// Profile or DM request has enabled stdout logging
			//if profile.Data.LogStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.LogStdout) {
			//	log.Info("Data movement operation output", "output", output)
			//}
			log.Info("Data movement operation output", "output", output)

			//// Profile or DM request has enabled storing stdout
			//if profile.Data.StoreStdout || (dm.Spec.UserConfig != nil && dm.Spec.UserConfig.StoreStdout) {
			//	dm.Status.Message = output
			//}
		}

		os.RemoveAll(filepath.Dir(mpiHostfile))

		//status := dm.Status.DeepCopy()

		//err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		//	dm := &nnfv1alpha6.NnfDataMovement{}
		//	if err := r.Get(ctx, req.NamespacedName, dm); err != nil {
		//		return client.IgnoreNotFound(err)
		//	}
		//
		//	// Ensure we have the latest CommandStatus from the progress goroutine
		//	cmdStatus.DeepCopyInto(status.CommandStatus)
		//	status.DeepCopyInto(&dm.Status)
		//
		//	return r.Status().Update(ctx, dm)
		//})
		//
		//if err != nil {
		//	log.Error(err, "failed to update dm status with completion")
		//	// TODO Add prometheus counter to track occurrences
		//}

		contextDelete()
	}()
}

// Set the DM's UserConfig options based on the incoming requests's options
func setUserConfig(dmreq DMRequest, dm *nnfv1alpha6.NnfDataMovement) {
	dm.Spec.UserConfig = &nnfv1alpha6.NnfDataMovementConfig{}
	dm.Spec.UserConfig.Dryrun = dmreq.Dryrun
	dm.Spec.UserConfig.MpirunOptions = dmreq.MpirunOptions
	dm.Spec.UserConfig.DcpOptions = dmreq.DcpOptions
	dm.Spec.UserConfig.LogStdout = dmreq.LogStdout
	dm.Spec.UserConfig.StoreStdout = dmreq.StoreStdout

	if dmreq.Slots >= 0 {
		dm.Spec.UserConfig.Slots = pointy.Int(int(dmreq.Slots))
	}
	if dmreq.MaxSlots >= 0 {
		dm.Spec.UserConfig.MaxSlots = pointy.Int(int(dmreq.MaxSlots))
	}
}

// createNnfNodeDataMovement creates an NnfDataMovement to be used with Lustre.
func (r *DriverRequest) createNnfDataMovement(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha2.ClientMountInfo, computeClientMount *dwsv1alpha2.ClientMount) (*nnfv1alpha6.NnfDataMovement, error) {

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

	dm := &nnfv1alpha6.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			// Be careful about how much you put into GenerateName.
			// The MPI operator will use the resulting name as a
			// prefix for its own names.
			GenerateName: nameBase,
			// Use the data movement namespace.
			Namespace: nnfv1alpha6.DataMovementNamespace,
			Labels: map[string]string{
				nnfv1alpha6.DataMovementInitiatorLabel: dmreq.ComputeName,
				nnfv1alpha6.DirectiveIndexLabel:        dwIndex,
			},
		},
		Spec: nnfv1alpha6.NnfDataMovementSpec{
			Source: &nnfv1alpha6.NnfDataMovementSpecSourceDestination{
				Path:             source,
				StorageReference: computeMountInfo.Device.DeviceReference.ObjectReference,
			},
			Destination: &nnfv1alpha6.NnfDataMovementSpecSourceDestination{
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

func getDirectiveIndexFromClientMount(object *dwsv1alpha2.ClientMount) (string, error) {
	// Find the DW index for our work.
	labels := object.GetLabels()
	if labels == nil {
		return "", fmt.Errorf("unable to find labels on compute ClientMount, namespaces=%s, name=%s", object.Namespace, object.Name)
	}

	dwIndex, found := labels[nnfv1alpha6.DirectiveIndexLabel]
	if !found {
		return "", fmt.Errorf("unable to find directive index label on compute ClientMount, namespace=%s name=%s", object.Namespace, object.Name)
	}

	return dwIndex, nil
}

// createNnfNodeDataMovement creates an NnfDataMovement to be used with GFS2.
func (r *DriverRequest) createNnfNodeDataMovement(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha2.ClientMountInfo) (*nnfv1alpha6.NnfDataMovement, error) {
	drvr := r.Drvr

	// Find the ClientMount for the rabbit.
	source, err := r.findRabbitRelativeSource(ctx, dmreq, computeMountInfo)
	if err != nil {
		return nil, err
	}

	dm := &nnfv1alpha6.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nodeNameBase,
			Namespace:    drvr.RabbitName, // Use the rabbit
			Labels: map[string]string{
				nnfv1alpha6.DataMovementInitiatorLabel: dmreq.ComputeName,
			},
		},
		Spec: nnfv1alpha6.NnfDataMovementSpec{
			Source: &nnfv1alpha6.NnfDataMovementSpecSourceDestination{
				Path:             source,
				StorageReference: computeMountInfo.Device.DeviceReference.ObjectReference,
			},
			Destination: &nnfv1alpha6.NnfDataMovementSpecSourceDestination{
				Path: dmreq.DestinationPath,
			},
		},
	}

	return dm, nil
}

func (r *DriverRequest) findRabbitRelativeSource(ctx context.Context, dmreq DMRequest, computeMountInfo *dwsv1alpha2.ClientMountInfo) (string, error) {

	drvr := r.Drvr
	// Now look up the client mount on this Rabbit node and find the compute initiator. We append the relative path
	// to this value resulting in the full path on the Rabbit.

	listOptions := []client.ListOption{
		client.InNamespace(drvr.RabbitName),
		client.MatchingLabels(map[string]string{
			dwsv1alpha2.WorkflowNameLabel:      dmreq.WorkflowName,
			dwsv1alpha2.WorkflowNamespaceLabel: dmreq.WorkflowNamespace,
		}),
	}

	clientMounts := &dwsv1alpha2.ClientMountList{}
	if err := drvr.Client.List(ctx, clientMounts, listOptions...); err != nil {
		return "", err
	}

	if len(clientMounts.Items) == 0 {
		return "", fmt.Errorf("no client mounts found for node '%s'", drvr.RabbitName)
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
func (r *DriverRequest) findComputeMountInfo(ctx context.Context, dmreq DMRequest) (*dwsv1alpha2.ClientMount, *dwsv1alpha2.ClientMountInfo, error) {

	drvr := r.Drvr
	listOptions := []client.ListOption{
		client.InNamespace(dmreq.ComputeName),
		client.MatchingLabels(map[string]string{
			dwsv1alpha2.WorkflowNameLabel:      dmreq.WorkflowName,
			dwsv1alpha2.WorkflowNamespaceLabel: dmreq.WorkflowNamespace,
		}),
	}

	clientMounts := &dwsv1alpha2.ClientMountList{}
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

func (r *DriverRequest) selectProfile(ctx context.Context, dmreq DMRequest) (*nnfv1alpha6.NnfDataMovementProfile, error) {
	drvr := r.Drvr
	profileName := dmreq.DMProfile
	ns := "nnf-system"

	// If a profile is named then verify that it exists.  Otherwise, verify that a default profile
	// can be found.
	if len(profileName) == 0 {
		NnfDataMovementProfiles := &nnfv1alpha6.NnfDataMovementProfileList{}
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

	profile := &nnfv1alpha6.NnfDataMovementProfile{
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
