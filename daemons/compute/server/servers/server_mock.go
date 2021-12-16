package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

// Mock server implements a simple Data Mover Server that accepts and completes all data movemement requests.
type mockServer struct {
	pb.UnimplementedRsyncDataMoverServer
}

func CreateMockServer(opts *ServerOptions) (*mockServer, error) {
	return &mockServer{}, nil
}

func (s *mockServer) Create(ctx context.Context, req *pb.RsyncDataMovementCreateRequest) (*pb.RsyncDataMovementCreateResponse, error) {
	return &pb.RsyncDataMovementCreateResponse{Uid: uuid.New().String()}, nil
}

func (s *mockServer) Status(ctx context.Context, req *pb.RsyncDataMovementStatusRequest) (*pb.RsyncDataMovementStatusResponse, error) {
	return &pb.RsyncDataMovementStatusResponse{
		State:   pb.RsyncDataMovementStatusResponse_COMPLETED,
		Message: fmt.Sprintf("The request %s completed successfully", req.Uid),
	}, nil
}
