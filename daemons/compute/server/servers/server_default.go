package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"

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
	namespace string
}

func CreateDefaultServer(opts *ServerOptions) (*defaultServer, error) {

	var config *rest.Config
	var err error

	switch opts.config {

	// The uncommon case is when the kube configuration is provided by an incluster
	// or user supplied kubeconfig; we typically won't support this case since the
	// access to the kube api server is over a service account.
	case KubeServerConfig:
		log.Printf("Using kubeconfig rest configuration")
		config, err = ctrl.GetConfig()
		if err != nil {
			return nil, err
		}

	default:
		log.Printf("Using default rest configuration")

		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) == 0 || len(port) == 0 {
			return nil, fmt.Errorf("kubernetes service host/port not defined")
		}

		tokenFile := os.Getenv("NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE")
		if len(tokenFile) == 0 {
			return nil, fmt.Errorf("nnf data movement service token not defined")
		}

		token, err := ioutil.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("nnf data movement service token failed to read")
		}

		certFile := os.Getenv("NNF_DATA_MOVEMENT_SERVICE_CERT_FILE")
		if len(certFile) == 0 {
			return nil, fmt.Errorf("nnf data movement service certificate file not defined")
		}

		if _, err := certutil.NewPool(certFile); err != nil {
			return nil, fmt.Errorf("nnf data movement service certificate invalid")
		}

		tlsClientConfig := rest.TLSClientConfig{}
		tlsClientConfig.CAFile = certFile

		config = &rest.Config{
			Host:            "https://" + net.JoinHostPort(host, port),
			TLSClientConfig: tlsClientConfig,
			BearerToken:     string(token),
			BearerTokenFile: tokenFile,
		}
	}

	client, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	// TODO: We need to create the NNFDataMovement CR on the associated rabbit namespace
	namespace := opts.nodename

	return &defaultServer{client: client, namespace: namespace}, nil
}

func (s *defaultServer) Create(ctx context.Context, req *pb.RsyncDataMovementCreateRequest) (*pb.RsyncDataMovementCreateResponse, error) {

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
		},
		Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
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
		return nil, err
	}

	stateMap := map[string]pb.RsyncDataMovementStatusResponse_State{
		nnfv1alpha1.DataMovementConditionTypeStarting: pb.RsyncDataMovementStatusResponse_STARTING,
		nnfv1alpha1.DataMovementConditionTypeRunning:  pb.RsyncDataMovementStatusResponse_RUNNING,
		nnfv1alpha1.DataMovementConditionTypeFinished: pb.RsyncDataMovementStatusResponse_COMPLETED,
	}

	state, ok := stateMap[rsync.Status.State]
	if !ok {
		return &pb.RsyncDataMovementStatusResponse{
				State:   pb.RsyncDataMovementStatusResponse_UNKNOWN,
				Message: fmt.Sprintf("State %s unknown", rsync.Status.State)},
			fmt.Errorf("failed to decode returned status")
	}

	return &pb.RsyncDataMovementStatusResponse{State: state, Message: rsync.Status.Status}, nil
}
