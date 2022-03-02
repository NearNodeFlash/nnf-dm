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

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"

	"github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/server/auth"
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
	pb.UnimplementedRsyncDataMoverServer

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

	// TODO: We need to create the NNFDataMovement CR on the associated rabbit namespace

	return &defaultServer{client: client, name: opts.name, namespace: opts.nodeName}, nil
}

func (s *defaultServer) Create(ctx context.Context, req *pb.RsyncDataMovementCreateRequest) (*pb.RsyncDataMovementCreateResponse, error) {

	userId, groupId, err := auth.GetAuthInfo(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.GetInitiator()) == 0 {
		req.Initiator = s.name
	}
	if len(req.GetTarget()) == 0 {
		req.Target = s.namespace
	}

Retry:
	// Create an ID - We could optionally perform a Get here and ensure that the UUID is not present, but the
	// chances of a random UUID colliding on a single NNF Node is close to zero.
	name := uuid.New()

	dm := &dmv1alpha1.RsyncNodeDataMovement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.String(),
			Namespace: req.GetTarget(),
			Labels: map[string]string{
				dmv1alpha1.OwnerLabelRsyncNodeDataMovement:          req.Workflow,
				dmv1alpha1.OwnerNamespaceLabelRsyncNodeDataMovement: req.Namespace,
			},
			Annotations: map[string]string{
				dmv1alpha1.OwnerLabelRsyncNodeDataMovement: req.Workflow + "/" + req.Namespace,
			},
		},
		Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
			Initiator:   req.GetInitiator(),
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

	return &pb.RsyncDataMovementCreateResponse{Uid: name.String()}, nil
}

func (s *defaultServer) Status(ctx context.Context, req *pb.RsyncDataMovementStatusRequest) (*pb.RsyncDataMovementStatusResponse, error) {

	rsync := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.namespace}, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.RsyncDataMovementStatusResponse{
				State:  pb.RsyncDataMovementStatusResponse_UNKNOWN_STATE,
				Status: pb.RsyncDataMovementStatusResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	if rsync.Status.StartTime.IsZero() {
		return &pb.RsyncDataMovementStatusResponse{
			State:  pb.RsyncDataMovementStatusResponse_PENDING,
			Status: pb.RsyncDataMovementStatusResponse_SUCCESS,
		}, nil
	}

	stateMap := map[string]pb.RsyncDataMovementStatusResponse_State{
		nnfv1alpha1.DataMovementConditionTypeStarting: pb.RsyncDataMovementStatusResponse_STARTING,
		nnfv1alpha1.DataMovementConditionTypeRunning:  pb.RsyncDataMovementStatusResponse_RUNNING,
		nnfv1alpha1.DataMovementConditionTypeFinished: pb.RsyncDataMovementStatusResponse_COMPLETED,
	}

	state, ok := stateMap[rsync.Status.State]
	if !ok {
		return &pb.RsyncDataMovementStatusResponse{
				State:   pb.RsyncDataMovementStatusResponse_UNKNOWN_STATE,
				Status:  pb.RsyncDataMovementStatusResponse_FAILED,
				Message: fmt.Sprintf("State %s unknown", rsync.Status.State)},
			fmt.Errorf("failed to decode returned state")
	}

	statusMap := map[string]pb.RsyncDataMovementStatusResponse_Status{
		nnfv1alpha1.DataMovementConditionReasonFailed:  pb.RsyncDataMovementStatusResponse_FAILED,
		nnfv1alpha1.DataMovementConditionReasonSuccess: pb.RsyncDataMovementStatusResponse_SUCCESS,
		nnfv1alpha1.DataMovementConditionReasonInvalid: pb.RsyncDataMovementStatusResponse_INVALID,
	}

	status, ok := statusMap[rsync.Status.Status]
	if !ok {
		return &pb.RsyncDataMovementStatusResponse{
				State:   state,
				Status:  pb.RsyncDataMovementStatusResponse_UNKNOWN_STATUS,
				Message: fmt.Sprintf("Status %s unknown", rsync.Status.Status)},
			fmt.Errorf("failed to decode returned status")
	}

	return &pb.RsyncDataMovementStatusResponse{State: state, Status: status, Message: rsync.Status.Message}, nil
}

func (s *defaultServer) Delete(ctx context.Context, req *pb.RsyncDataMovementDeleteRequest) (*pb.RsyncDataMovementDeleteResponse, error) {

	rsync := &dmv1alpha1.RsyncNodeDataMovement{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.Uid, Namespace: s.namespace}, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.RsyncDataMovementDeleteResponse{
				Status: pb.RsyncDataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	if rsync.Status.State != nnfv1alpha1.DataMovementConditionTypeFinished {
		return &pb.RsyncDataMovementDeleteResponse{
			Status: pb.RsyncDataMovementDeleteResponse_ACTIVE,
		}, nil
	}

	if err := s.client.Delete(ctx, rsync); err != nil {
		if errors.IsNotFound(err) {
			return &pb.RsyncDataMovementDeleteResponse{
				Status: pb.RsyncDataMovementDeleteResponse_NOT_FOUND,
			}, nil
		}

		return nil, err
	}

	return &pb.RsyncDataMovementDeleteResponse{
		Status: pb.RsyncDataMovementDeleteResponse_DELETED,
	}, nil
}
