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

package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"

	dmctrl "github.com/NearNodeFlash/nnf-dm/controllers"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"

	"github.com/NearNodeFlash/nnf-dm/daemons/compute/server/auth"
)

var (
	scheme = runtime.NewScheme()

	// This naming prefix is used during status calls to indicate that
	// this is an NnfDataMovement rather than an RsyncNodeDataMovement.
	nameBase = "nnfdm"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dwsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(dmv1alpha1.AddToScheme(scheme))
	utilruntime.Must(nnfv1alpha1.AddToScheme(scheme))
	utilruntime.Must(lusv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

type defaultServer struct {
	pb.UnimplementedDataMoverServer

	config    *rest.Config
	client    client.Client
	name      string
	namespace string

	cond        *sync.Cond
	completions map[string]struct{}
}

func CreateDefaultServer(opts *ServerOptions) (*defaultServer, error) {

	var config *rest.Config
	var err error

	if len(opts.host) == 0 && len(opts.port) == 0 {
		log.Printf("Using kubeconfig rest configuration")
		config, err = ctrl.GetConfig()
		if err != nil {
			return nil, err
		}
	} else {
		log.Printf("Using default rest configuration")

		if len(opts.host) == 0 || len(opts.port) == 0 {
			return nil, fmt.Errorf("kubernetes service host/port not defined")
		}

		if len(opts.tokenFile) == 0 {
			return nil, fmt.Errorf("nnf data movement service token not defined")
		}

		token, err := ioutil.ReadFile(opts.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("nnf data movement service token failed to read")
		}

		if len(opts.certFile) == 0 {
			return nil, fmt.Errorf("nnf data movement service certificate file not defined")
		}

		if _, err := certutil.NewPool(opts.certFile); err != nil {
			return nil, fmt.Errorf("nnf data movement service certificate invalid")
		}

		tlsClientConfig := rest.TLSClientConfig{}
		tlsClientConfig.CAFile = opts.certFile

		config = &rest.Config{
			Host:            "https://" + net.JoinHostPort(opts.host, opts.port),
			TLSClientConfig: tlsClientConfig,
			BearerToken:     string(token),
			BearerTokenFile: opts.tokenFile,
		}
	}

	client, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &defaultServer{
		config:      config,
		client:      client,
		name:        opts.name,
		namespace:   opts.nodeName,
		cond:        sync.NewCond(&sync.Mutex{}),
		completions: make(map[string]struct{}),
	}, nil
}

func (s *defaultServer) StartManager() error {
	mgr, err := ctrl.NewManager(s.config, ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
		Namespace:          s.namespace,
	})
	if err != nil {
		return err
	}

	if err := s.setupWithManager(mgr); err != nil {
		return err
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}

	return nil
}

// Setup two managers for watching the individual data movement type resources. They behave
// similarily, performing a reconcile only for updates to this node
func (s *defaultServer) setupWithManager(mgr ctrl.Manager) error {

	p := predicate.Funcs{
		CreateFunc: func(ce event.CreateEvent) bool { return false },
		UpdateFunc: func(ue event.UpdateEvent) bool {
			if initiator, _ := ue.ObjectNew.GetLabels()[dmctrl.InitiatorLabel]; initiator == s.name {
				return true
			}
			return false
		},
		DeleteFunc: func(de event.DeleteEvent) bool { return false },
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&dmv1alpha1.RsyncNodeDataMovement{}, builder.WithPredicates(p)).
		Complete(&rsyncNodeReconciler{s})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&nnfv1alpha1.NnfDataMovement{}, builder.WithPredicates(p)).
		Complete(&dataMovementReconciler{s})
	if err != nil {
		return err
	}

	return nil
}

type rsyncNodeReconciler struct {
	server *defaultServer
}

func (r *rsyncNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	rsync := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := r.server.client.Get(ctx, req.NamespacedName, rsync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rsync.DeletionTimestamp.IsZero() {
		r.server.deleteCompletion(rsync.Name)
		return ctrl.Result{}, nil
	}

	if !rsync.Status.EndTime.IsZero() {
		r.server.notifyCompletion(req.Name)
	}

	return ctrl.Result{}, nil
}

type dataMovementReconciler struct {
	server *defaultServer
}

func (r *dataMovementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := r.server.client.Get(ctx, req.NamespacedName, dm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !dm.DeletionTimestamp.IsZero() {
		r.server.deleteCompletion(dm.Name)
		return ctrl.Result{}, nil
	}

	if !dm.Status.EndTime.IsZero() {
		r.server.notifyCompletion(req.Name)
	}

	return ctrl.Result{}, nil
}

func (s *defaultServer) Create(ctx context.Context, req *pb.DataMovementCreateRequest) (*pb.DataMovementCreateResponse, error) {

	userId, groupId, err := auth.GetAuthInfo(ctx)
	if err != nil {
		return nil, err
	}

	computeClientMount, computeMountInfo, err := s.findComputeMountInfo(ctx, req)
	if err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	if computeMountInfo.Type == "lustre" {
		return s.createNnfDataMovement(ctx, req, userId, groupId, computeMountInfo, computeClientMount)
	} else {
		return s.createRsyncNodeDataMovement(ctx, req, userId, groupId, computeMountInfo)
	}
}

func getDirectiveIndexFromClientMount(object *dwsv1alpha1.ClientMount) (string, error) {
	// Find the DW index for our work.
	labels := object.GetLabels()
	if labels == nil {
		return "", fmt.Errorf("Unable to find labels on compute ClientMount, namespaces=%s, name=%s", object.Namespace, object.Name)
	}

	dwIndex, found := labels[nnfv1alpha1.DirectiveIndexLabel]
	if !found {
		return "", fmt.Errorf("Unable to find directive index label on compute ClientMount, namespace=%s name=%s", object.Namespace, object.Name)
	}

	return dwIndex, nil
}

func (s *defaultServer) createNnfDataMovement(ctx context.Context, req *pb.DataMovementCreateRequest, userId uint32, groupId uint32, computeMountInfo *dwsv1alpha1.ClientMountInfo, computeClientMount *dwsv1alpha1.ClientMount) (*pb.DataMovementCreateResponse, error) {

	var dwIndex string
	if dw, err := getDirectiveIndexFromClientMount(computeClientMount); err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	} else {
		dwIndex = dw
	}

	lustrefs, err := s.findDestinationLustreFilesystem(ctx, req.GetDestination())
	if err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	// We don't have the actual NnfDataMovement parent available, but we know the name
	// and the namespace because they will match the workflow's name and namespace.
	parentDm := &nnfv1alpha1.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Workflow,
			Namespace: req.Namespace,
		},
	}

	dm := &nnfv1alpha1.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			// Be careful about how much you put into GenerateName.
			// The MPI operator will use the resulting name as a
			// prefix for its own names.
			GenerateName: fmt.Sprintf("%s-", nameBase),
			// Use the compute's namespace.
			Namespace: s.name,
			Labels: map[string]string{
				dmctrl.InitiatorLabel:           s.name,
				nnfv1alpha1.DirectiveIndexLabel: dwIndex,
			},
		},
		Spec: nnfv1alpha1.NnfDataMovementSpec{
			Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
				Path: req.GetDestination(),
				Storage: &corev1.ObjectReference{
					Kind:      reflect.TypeOf(*lustrefs).Name(),
					Namespace: lustrefs.Namespace,
					Name:      lustrefs.Name,
				},
			},
			Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
				Path:    "/",
				Storage: &computeMountInfo.Device.DeviceReference.ObjectReference,
			},
			UserId:  userId,
			GroupId: groupId,
		},
	}

	dwsv1alpha1.AddOwnerLabels(dm, parentDm)

	if err := s.client.Create(ctx, dm, &client.CreateOptions{}); err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	return &pb.DataMovementCreateResponse{
		Uid:    dm.GetName(),
		Status: pb.DataMovementCreateResponse_CREATED,
	}, nil
}

func (s *defaultServer) createRsyncNodeDataMovement(ctx context.Context, req *pb.DataMovementCreateRequest, userId uint32, groupId uint32, computeMountInfo *dwsv1alpha1.ClientMountInfo) (*pb.DataMovementCreateResponse, error) {
	// Find the ClientMount for the rabbit.
	source, err := s.findRabbitRelativeSource(ctx, computeMountInfo, req)
	if err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	dm := &dmv1alpha1.RsyncNodeDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", req.Namespace, req.Workflow),
			Namespace:    s.namespace,
			Labels: map[string]string{
				dmctrl.InitiatorLabel: s.name,
			},
		},
		Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
			Initiator:   s.name,
			Source:      source,
			Destination: req.GetDestination(),
			UserId:      userId,
			GroupId:     groupId,
			DryRun:      req.GetDryrun(),
		},
	}

	// We don't have the actual NnfDataMovement parent available, but we know the name
	// and the namespace because they will match the workflow's name and namespace. Create
	// a fake parent here to use for the AddOwnerLabels call.
	parent := &nnfv1alpha1.NnfDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Workflow,
			Namespace: req.Namespace,
		},
	}

	dwsv1alpha1.AddOwnerLabels(dm, parent)

	if err := s.client.Create(ctx, dm, &client.CreateOptions{}); err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	return &pb.DataMovementCreateResponse{
		Uid:    dm.GetName(),
		Status: pb.DataMovementCreateResponse_CREATED,
	}, nil
}

func (s *defaultServer) List(ctx context.Context, req *pb.DataMovementListRequest) (*pb.DataMovementListResponse, error) {

	// Only get the DMs that match the workflow and workflow namespace
	opts := []client.ListOption{
		client.MatchingLabels{dwsv1alpha1.OwnerNamespaceLabel: req.Namespace},
		client.MatchingLabels{dwsv1alpha1.OwnerNameLabel: req.Workflow},
	}

	list := dmv1alpha1.RsyncNodeDataMovementList{}
	if err := s.client.List(ctx, &list, opts...); err != nil {
		return nil, err
	}

	rsyncdm_uids := make([]string, 0)
	for i := 0; i < len(list.Items); i++ {
		// extract the uid (name) and add to the list
		rsyncdm_uids = append(rsyncdm_uids, list.Items[i].ObjectMeta.Name)
	}

	return &pb.DataMovementListResponse{
		Uids: rsyncdm_uids,
	}, nil
}

func (s *defaultServer) Status(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {

	if strings.HasPrefix(req.Uid, nameBase) {
		return s.nnfDataMovementStatus(ctx, req)
	} else {
		return s.rsyncNodeDataMovementStatus(ctx, req)
	}
}

func (s *defaultServer) rsyncNodeDataMovementStatus(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {

	rsync := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.namespace}, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementStatusResponse{
				State:  pb.DataMovementStatusResponse_UNKNOWN_STATE,
				Status: pb.DataMovementStatusResponse_NOT_FOUND,
			}, nil
		}
		return nil, err
	}

	if rsync.Status.EndTime.IsZero() && req.MaxWaitTime != 0 {

		s.waitForCompletionOrTimeout(req)

		if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.namespace}, rsync); err != nil {
			if errors.IsNotFound(err) {
				return &pb.DataMovementStatusResponse{
					State:  pb.DataMovementStatusResponse_UNKNOWN_STATE,
					Status: pb.DataMovementStatusResponse_NOT_FOUND,
				}, nil
			}

			return nil, err
		}
	}

	if rsync.Status.StartTime.IsZero() {
		return &pb.DataMovementStatusResponse{
			State:  pb.DataMovementStatusResponse_PENDING,
			Status: pb.DataMovementStatusResponse_SUCCESS,
		}, nil
	}

	stateMap := map[string]pb.DataMovementStatusResponse_State{
		"": pb.DataMovementStatusResponse_UNKNOWN_STATE,
		nnfv1alpha1.DataMovementConditionTypeStarting: pb.DataMovementStatusResponse_STARTING,
		nnfv1alpha1.DataMovementConditionTypeRunning:  pb.DataMovementStatusResponse_RUNNING,
		nnfv1alpha1.DataMovementConditionTypeFinished: pb.DataMovementStatusResponse_COMPLETED,
	}

	state, ok := stateMap[rsync.Status.State]
	if !ok {
		return &pb.DataMovementStatusResponse{
				State:   pb.DataMovementStatusResponse_UNKNOWN_STATE,
				Status:  pb.DataMovementStatusResponse_FAILED,
				Message: fmt.Sprintf("State %s unknown", rsync.Status.State)},
			fmt.Errorf("failed to decode returned state")
	}

	statusMap := map[string]pb.DataMovementStatusResponse_Status{
		"": pb.DataMovementStatusResponse_UNKNOWN_STATUS,
		nnfv1alpha1.DataMovementConditionReasonFailed:  pb.DataMovementStatusResponse_FAILED,
		nnfv1alpha1.DataMovementConditionReasonSuccess: pb.DataMovementStatusResponse_SUCCESS,
		nnfv1alpha1.DataMovementConditionReasonInvalid: pb.DataMovementStatusResponse_INVALID,
	}

	status, ok := statusMap[rsync.Status.Status]
	if !ok {
		return &pb.DataMovementStatusResponse{
				State:   state,
				Status:  pb.DataMovementStatusResponse_UNKNOWN_STATUS,
				Message: fmt.Sprintf("Status %s unknown", rsync.Status.Status)},
			fmt.Errorf("failed to decode returned status")
	}

	return &pb.DataMovementStatusResponse{State: state, Status: status, Message: rsync.Status.Message}, nil
}

func (s *defaultServer) nnfDataMovementStatus(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.name}, dm); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementStatusResponse{
				State:  pb.DataMovementStatusResponse_UNKNOWN_STATE,
				Status: pb.DataMovementStatusResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	if dm.Status.EndTime.IsZero() && req.MaxWaitTime != 0 {

		s.waitForCompletionOrTimeout(req)

		if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.name}, dm); err != nil {
			if errors.IsNotFound(err) {
				return &pb.DataMovementStatusResponse{
					State:  pb.DataMovementStatusResponse_UNKNOWN_STATE,
					Status: pb.DataMovementStatusResponse_NOT_FOUND,
				}, nil
			}

			return nil, err
		}
	}

	if dm.Status.StartTime.IsZero() {
		return &pb.DataMovementStatusResponse{
			State:  pb.DataMovementStatusResponse_PENDING,
			Status: pb.DataMovementStatusResponse_SUCCESS,
		}, nil
	}

	stateMap := map[string]pb.DataMovementStatusResponse_State{
		"": pb.DataMovementStatusResponse_UNKNOWN_STATE,
		nnfv1alpha1.DataMovementConditionTypeStarting: pb.DataMovementStatusResponse_STARTING,
		nnfv1alpha1.DataMovementConditionTypeRunning:  pb.DataMovementStatusResponse_RUNNING,
		nnfv1alpha1.DataMovementConditionTypeFinished: pb.DataMovementStatusResponse_COMPLETED,
	}

	state, ok := stateMap[dm.Status.State]
	if !ok {
		return &pb.DataMovementStatusResponse{
				State:   pb.DataMovementStatusResponse_UNKNOWN_STATE,
				Status:  pb.DataMovementStatusResponse_FAILED,
				Message: fmt.Sprintf("State %s unknown", dm.Status.State)},
			fmt.Errorf("failed to decode returned state")
	}

	statusMap := map[string]pb.DataMovementStatusResponse_Status{
		"": pb.DataMovementStatusResponse_UNKNOWN_STATUS,
		nnfv1alpha1.DataMovementConditionReasonFailed:  pb.DataMovementStatusResponse_FAILED,
		nnfv1alpha1.DataMovementConditionReasonSuccess: pb.DataMovementStatusResponse_SUCCESS,
		nnfv1alpha1.DataMovementConditionReasonInvalid: pb.DataMovementStatusResponse_INVALID,
	}

	status, ok := statusMap[dm.Status.Status]
	if !ok {
		return &pb.DataMovementStatusResponse{
				State:   state,
				Status:  pb.DataMovementStatusResponse_UNKNOWN_STATUS,
				Message: fmt.Sprintf("Status %s unknown", dm.Status.Status)},
			fmt.Errorf("failed to decode returned status")
	}

	return &pb.DataMovementStatusResponse{State: state, Status: status, Message: dm.Status.Message}, nil
}

func (s *defaultServer) notifyCompletion(name string) {
	s.cond.L.Lock()
	s.completions[name] = struct{}{}
	s.cond.L.Unlock()

	s.cond.Broadcast()
}

func (s *defaultServer) deleteCompletion(name string) {
	s.cond.L.Lock()
	delete(s.completions, name)
	s.cond.L.Unlock()
}

func (s *defaultServer) waitForCompletionOrTimeout(req *pb.DataMovementStatusRequest) {
	timeout := false
	complete := make(chan struct{}, 1)

	// Start a go routine that waits for the completion of this status request or for timeout
	go func() {

		s.cond.L.Lock()
		for true {
			if _, found := s.completions[req.Uid]; found || timeout {
				break
			}

			s.cond.Wait()
		}

		s.cond.L.Unlock()

		complete <- struct{}{}
	}()

	var maxWaitTimeDuration time.Duration
	if req.MaxWaitTime < 0 || time.Duration(req.MaxWaitTime) > math.MaxInt64/time.Second {
		maxWaitTimeDuration = math.MaxInt64
	} else {
		maxWaitTimeDuration = time.Duration(req.MaxWaitTime) * time.Second
	}

	// Wait for completion signal or timeout
	select {
	case <-complete:
	case <-time.After(maxWaitTimeDuration):
		timeout = true
		s.cond.Broadcast() // Wake up everyone (including myself) to close out the running go routine
	}
}

func (s *defaultServer) Delete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {

	if strings.HasPrefix(req.Uid, nameBase) {
		return s.nnfDataMovementDelete(ctx, req)
	} else {
		return s.rsyncNodeDataMovementDelete(ctx, req)
	}
}

func (s *defaultServer) rsyncNodeDataMovementDelete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {

	rsync := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.namespace}, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementDeleteResponse{
				Status: pb.DataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	if rsync.Status.State != nnfv1alpha1.DataMovementConditionTypeFinished {
		return &pb.DataMovementDeleteResponse{
			Status: pb.DataMovementDeleteResponse_ACTIVE,
		}, nil
	}

	if err := s.client.Delete(ctx, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementDeleteResponse{
				Status: pb.DataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	return &pb.DataMovementDeleteResponse{
		Status: pb.DataMovementDeleteResponse_DELETED,
	}, nil
}

func (s *defaultServer) nnfDataMovementDelete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {

	dm := &nnfv1alpha1.NnfDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.name}, dm); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementDeleteResponse{
				Status: pb.DataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	if dm.Status.State != nnfv1alpha1.DataMovementConditionTypeFinished {
		return &pb.DataMovementDeleteResponse{
			Status: pb.DataMovementDeleteResponse_ACTIVE,
		}, nil
	}

	if err := s.client.Delete(ctx, dm); err != nil {
		if errors.IsNotFound(err) {
			return &pb.DataMovementDeleteResponse{
				Status: pb.DataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	return &pb.DataMovementDeleteResponse{
		Status: pb.DataMovementDeleteResponse_DELETED,
	}, nil
}

func (s *defaultServer) findDestinationLustreFilesystem(ctx context.Context, dest string) (*lusv1alpha1.LustreFileSystem, error) {

	if !filepath.IsAbs(dest) {
		return nil, fmt.Errorf("Destination must be an absolute path")
	}
	origDest := dest
	if !strings.HasSuffix(dest, "/") {
		dest += "/"
	}

	lustrefsList := &lusv1alpha1.LustreFileSystemList{}
	if err := s.client.List(ctx, lustrefsList); err != nil {
		return nil, fmt.Errorf("Unable to list LustreFileSystem resources: %s", err.Error())
	}
	if len(lustrefsList.Items) == 0 {
		return nil, fmt.Errorf("No LustreFileSystem resources found")
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

	return nil, fmt.Errorf("Unable to find a LustreFileSystem resource matching %s", origDest)
}

func (s *defaultServer) findRabbitRelativeSource(ctx context.Context, computeMountInfo *dwsv1alpha1.ClientMountInfo, req *pb.DataMovementCreateRequest) (string, error) {

	// Now look up the client mount on this Rabbit node and find the compute initiator. We append the relative path
	// to this value resulting in the full path on the Rabbit.

	listOptions := []client.ListOption{
		client.InNamespace(s.namespace),
		client.MatchingLabels(map[string]string{
			dwsv1alpha1.WorkflowNameLabel:      req.Workflow,
			dwsv1alpha1.WorkflowNamespaceLabel: req.Namespace,
		}),
	}

	clientMounts := &dwsv1alpha1.ClientMountList{}
	if err := s.client.List(ctx, clientMounts, listOptions...); err != nil {
		return "", err
	}

	if len(clientMounts.Items) == 0 {
		return "", fmt.Errorf("No client mounts found on node '%s'", s.namespace)
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if *computeMountInfo.Device.DeviceReference == *mount.Device.DeviceReference {
				return mount.MountPath + strings.TrimPrefix(req.Source, computeMountInfo.MountPath), nil
			}
		}
	}

	return "", fmt.Errorf("Initiator '%s' not found in list of client mounts residing on '%s'", s.name, s.namespace)
}

// Look up the client mounts on this node to find the compute relative mount path. The "spec.Source" must be
// prefixed with a mount path in the list of mounts. Once we find this mount, we can strip out the prefix and
// are left with the relative path.
func (s *defaultServer) findComputeMountInfo(ctx context.Context, req *pb.DataMovementCreateRequest) (*dwsv1alpha1.ClientMount, *dwsv1alpha1.ClientMountInfo, error) {

	listOptions := []client.ListOption{
		client.InNamespace(s.name),
		client.MatchingLabels(map[string]string{
			dwsv1alpha1.WorkflowNameLabel:      req.Workflow,
			dwsv1alpha1.WorkflowNamespaceLabel: req.Namespace,
		}),
	}

	clientMounts := &dwsv1alpha1.ClientMountList{}
	if err := s.client.List(ctx, clientMounts, listOptions...); err != nil {
		return nil, nil, err
	}

	if len(clientMounts.Items) == 0 {
		return nil, nil, fmt.Errorf("No client mounts found on node '%s'", s.name)
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if strings.HasPrefix(req.GetSource(), mount.MountPath) {
				if mount.Device.DeviceReference == nil && mount.Device.Type != "lustre" {
					return nil, nil, fmt.Errorf("ClientMount %s/%s: Source path '%s' does not have device reference", clientMount.Namespace, clientMount.Name, req.GetSource())
				}

				return &clientMount, &mount, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("Source path '%s' not found in list of client mounts", req.GetSource())
}
