package main

import (
	"log"
	"net"
	"os"

	"google.golang.org/grpc"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"

	"github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/server/auth"
	server "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/server/servers"
)

const (
	socketAddr = "/tmp/nnf.sock"
)

func main() {

	os.Remove(socketAddr)

	lis, err := net.Listen("unix", socketAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	server, err := server.Create(server.GetOptions())
	if err != nil {
		log.Fatalf("Failed to create data mover server: %v", err)
	}

	s := grpc.NewServer(grpc.Creds(&auth.ServerAuthCredentials{}))
	pb.RegisterRsyncDataMoverServer(s, server)

	log.Printf("Server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
