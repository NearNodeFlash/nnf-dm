package server

import (
	"flag"
	"os"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

type ServerOptions struct {
	host      string
	port      string
	tokenFile string
	certFile  string

	nodeName  string
	simulated bool
}

func GetOptions() (*ServerOptions, error) {
	opts := ServerOptions{
		host:      os.Getenv("KUBERNETES_SERVICE_HOST"),
		port:      os.Getenv("KUBERNETES_SERVICE_PORT"),
		tokenFile: os.Getenv("NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE"),
		certFile:  os.Getenv("NNF_DATA_MOVEMENT_SERVICE_CERT_FILE"),
		nodeName:  os.Getenv("NNF_NODE_NAME"),
		simulated: false,
	}

	flag.StringVar(&opts.host, "kubernetes-service-host", opts.host, "Kubernetes service host address")
	flag.StringVar(&opts.port, "kubernetes-service-port", opts.port, "Kubernetes service port number")
	flag.StringVar(&opts.tokenFile, "nnf-data-movement-service-token-file", opts.tokenFile, "Path to the NNF data movement service token")
	flag.StringVar(&opts.certFile, "nnf-data-movement-service-cert-file", opts.certFile, "Path to the NNF data movement service certificate")
	flag.StringVar(&opts.nodeName, "nnf-node-name", opts.nodeName, "NNF node name that should handle the data movement request")
	flag.BoolVar(&opts.simulated, "simulated", opts.simulated, "Run in simulation mode where no requests are sent to the server")
	flag.Parse()
	return &opts, nil
}

func Create(opts *ServerOptions) (pb.RsyncDataMoverServer, error) {
	if opts.simulated {
		return CreateSimulatedServer(opts)
	}

	return CreateDefaultServer(opts)
}
