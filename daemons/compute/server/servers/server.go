package server

import (
	"flag"
	"os"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

const (
	DefaultServerConfig = "default"
	KubeServerConfig    = "kube"
	MockServerConfig    = "mock"
)

type ServerOptions struct {
	config   string
	nodename string
}

func GetOptions() *ServerOptions {
	opts := ServerOptions{
		config:   DefaultServerConfig,
		nodename: os.Getenv("NNF_NODE_NAME"),
	}

	flag.StringVar(&opts.config, "config", opts.config, "server configuration to use (default, kube, mock)")
	flag.StringVar(&opts.nodename, "nodename", opts.nodename, "nnf node name that should handle the data movement request")
	flag.Parse()

	return &opts
}

func Create(opts *ServerOptions) (pb.RsyncDataMoverServer, error) {
	switch opts.config {
	case DefaultServerConfig, KubeServerConfig:
		return CreateDefaultServer(opts)
	case MockServerConfig:
		return CreateMockServer(opts)
	}

	return &pb.UnimplementedRsyncDataMoverServer{}, nil
}
