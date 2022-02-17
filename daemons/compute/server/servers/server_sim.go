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

	requests map[uuid.UUID]interface{}
}

func CreateSimulatedServer(opts *ServerOptions) (*simulatedServer, error) {
	return &simulatedServer{}, nil
}

func (s *simulatedServer) Create(ctx context.Context, req *pb.RsyncDataMovementCreateRequest) (*pb.RsyncDataMovementCreateResponse, error) {
	uid := uuid.New()

	s.requests[uid] = nil

	return &pb.RsyncDataMovementCreateResponse{Uid: uid.String()}, nil
}

func (s *simulatedServer) Status(ctx context.Context, req *pb.RsyncDataMovementStatusRequest) (*pb.RsyncDataMovementStatusResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.RsyncDataMovementStatusResponse{
			State:   pb.RsyncDataMovementStatusResponse_UNKNOWN_STATE,
			Status:  pb.RsyncDataMovementStatusResponse_INVALID,
			Message: fmt.Sprintf("Request %s is invalid", req.Uid),
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.RsyncDataMovementStatusResponse{
			State:   pb.RsyncDataMovementStatusResponse_UNKNOWN_STATE,
			Status:  pb.RsyncDataMovementStatusResponse_NOT_FOUND,
			Message: fmt.Sprintf("Request %s not found", req.Uid),
		}, nil
	}

	return &pb.RsyncDataMovementStatusResponse{
		State:   pb.RsyncDataMovementStatusResponse_COMPLETED,
		Status:  pb.RsyncDataMovementStatusResponse_SUCCESS,
		Message: fmt.Sprintf("Request %s completed successfully", req.Uid),
	}, nil
}

func (s *simulatedServer) Delete(ctx context.Context, req *pb.RsyncDataMovementDeleteRequest) (*pb.RsyncDataMovementDeleteResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.RsyncDataMovementDeleteResponse{
			Status: pb.RsyncDataMovementDeleteResponse_INVALID,
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.RsyncDataMovementDeleteResponse{
			Status: pb.RsyncDataMovementDeleteResponse_NOT_FOUND,
		}, nil
	}

	delete(s.requests, uid)

	return &pb.RsyncDataMovementDeleteResponse{
		Status: pb.RsyncDataMovementDeleteResponse_DELETED,
	}, nil
}
