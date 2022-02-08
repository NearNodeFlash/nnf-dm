package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

// Simulated server implements a simple Data Mover Server that accepts and completes all data movemement requests.
type simulatedServer struct {
	pb.UnimplementedRsyncDataMoverServer
}

func CreateSimulatedServer(opts *ServerOptions) (*simulatedServer, error) {
	return &simulatedServer{}, nil
}

func (s *simulatedServer) Create(ctx context.Context, req *pb.RsyncDataMovementCreateRequest) (*pb.RsyncDataMovementCreateResponse, error) {
	return &pb.RsyncDataMovementCreateResponse{Uid: uuid.New().String()}, nil
}

func (s *simulatedServer) Status(ctx context.Context, req *pb.RsyncDataMovementStatusRequest) (*pb.RsyncDataMovementStatusResponse, error) {
	return &pb.RsyncDataMovementStatusResponse{
		State:   pb.RsyncDataMovementStatusResponse_COMPLETED,
		Status:  pb.RsyncDataMovementStatusResponse_SUCCESS,
		Message: fmt.Sprintf("The request %s completed successfully", req.Uid),
	}, nil
}
