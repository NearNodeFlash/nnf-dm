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

Retry:
	// Create an ID - We could optionally perform a Get here and ensure that the UUID is not present, but the
	// chances of a random UUID colliding on a single NNF Node is close to zero.
	name := uuid.New()

	dm := &dmv1alpha1.RsyncNodeDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.String(),
			Namespace: s.namespace,
			Labels: map[string]string{
				dmv1alpha1.OwnerLabelRsyncNodeDataMovement:          req.Workflow,
				dmv1alpha1.OwnerNamespaceLabelRsyncNodeDataMovement: req.Namespace,
			},
			Annotations: map[string]string{
				dmv1alpha1.OwnerLabelRsyncNodeDataMovement: req.Workflow + "/" + req.Namespace,
			},
		},
		Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
			Initiator:   s.name,
			Source:      req.GetSource(),
			Destination: req.GetDestination(),
			UserId:      userId,
			GroupId:     groupId,
			DryRun:      req.GetDryrun(),
		},
	}

	if err := s.client.Create(ctx, dm, &client.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			goto Retry
		}

		return nil, err
	}

	return &pb.DataMovementCreateResponse{Uid: name.String()}, nil
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
