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
	"net"
	"strings"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/api"

	"github.com/NearNodeFlash/nnf-dm/daemons/compute/server/auth"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dwsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(dmv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

type defaultServer struct {
	pb.UnimplementedDataMoverServer

	client    client.Client
	name      string
	namespace string
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

	return &defaultServer{client: client, name: opts.name, namespace: opts.nodeName}, nil
}

func (s *defaultServer) Create(ctx context.Context, req *pb.DataMovementCreateRequest) (*pb.DataMovementCreateResponse, error) {

	userId, groupId, err := auth.GetAuthInfo(ctx)
	if err != nil {
		return nil, err
	}

	source, err := s.findRabbitRelativeSource(ctx, req)
	if err != nil {
		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

Retry:
	// Create an ID - We could optionally perform a Get here and ensure that the UUID is not present, but the
	// chances of a random UUID colliding on a single NNF Node is close to zero.
	name := uuid.New()

	dm := &dmv1alpha1.RsyncNodeDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.String(),
			Namespace: s.namespace,
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
		if errors.IsAlreadyExists(err) {
			goto Retry
		}

		return &pb.DataMovementCreateResponse{
			Status:  pb.DataMovementCreateResponse_FAILED,
			Message: err.Error(),
		}, nil
	}

	return &pb.DataMovementCreateResponse{
		Uid:    name.String(),
		Status: pb.DataMovementCreateResponse_CREATED,
	}, nil
}

func (s *defaultServer) Status(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {

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

func (s *defaultServer) Delete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {

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

func (s *defaultServer) findRabbitRelativeSource(ctx context.Context, req *pb.DataMovementCreateRequest) (string, error) {

	computeMountInfo, err := s.findComputeMountInfo(ctx, req)
	if err != nil {
		return "", err
	}

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
func (s *defaultServer) findComputeMountInfo(ctx context.Context, req *pb.DataMovementCreateRequest) (*dwsv1alpha1.ClientMountInfo, error) {

	listOptions := []client.ListOption{
		client.InNamespace(s.name),
		client.MatchingLabels(map[string]string{
			dwsv1alpha1.WorkflowNameLabel:      req.Workflow,
			dwsv1alpha1.WorkflowNamespaceLabel: req.Namespace,
		}),
	}

	clientMounts := &dwsv1alpha1.ClientMountList{}
	if err := s.client.List(ctx, clientMounts, listOptions...); err != nil {
		return nil, err
	}

	if len(clientMounts.Items) == 0 {
		return nil, fmt.Errorf("No client mounts found on node '%s'", s.name)
	}

	for _, clientMount := range clientMounts.Items {
		for _, mount := range clientMount.Spec.Mounts {
			if strings.HasPrefix(req.GetSource(), mount.MountPath) {
				if mount.Device.DeviceReference == nil {
					return nil, fmt.Errorf("Source path '%s' does not have device reference", req.GetSource())
				}

				return &mount, nil
			}
		}
	}

	return nil, fmt.Errorf("Source path '%s' not found in list of client mounts", req.GetSource())
}
