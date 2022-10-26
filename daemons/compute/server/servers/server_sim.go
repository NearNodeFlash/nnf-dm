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
	"time"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"
	"github.com/google/uuid"
)

// Simulated server implements a simple Data Mover Server that accepts and completes all data movemement requests.
type simulatedServer struct {
	pb.UnimplementedDataMoverServer

	requests map[uuid.UUID]interface{}
}

func CreateSimulatedServer(opts *ServerOptions) (*simulatedServer, error) {
	return &simulatedServer{requests: make(map[uuid.UUID]interface{})}, nil
}

func (s *simulatedServer) StartManager() error {
	return nil
}

func (s *simulatedServer) Create(ctx context.Context, req *pb.DataMovementCreateRequest) (*pb.DataMovementCreateResponse, error) {
	uid := uuid.New()

	s.requests[uid] = nil

	return &pb.DataMovementCreateResponse{Uid: uid.String()}, nil
}

var statusCount int = 1

func (s *simulatedServer) Status(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		statusCount = 1
		return &pb.DataMovementStatusResponse{
			State:         pb.DataMovementStatusResponse_UNKNOWN_STATE,
			Status:        pb.DataMovementStatusResponse_INVALID,
			Message:       fmt.Sprintf("Request %s is invalid", req.Uid),
			CommandStatus: nil,
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		statusCount = 1
		return &pb.DataMovementStatusResponse{
			State:         pb.DataMovementStatusResponse_UNKNOWN_STATE,
			Status:        pb.DataMovementStatusResponse_NOT_FOUND,
			Message:       fmt.Sprintf("Request %s not found", req.Uid),
			CommandStatus: nil,
		}, nil
	}

	resp := pb.DataMovementStatusResponse{}
	resp.CommandStatus = &pb.DataMovementCommandStatus{}
	cmd := "mpirun -np 1 dcp --progress 1 src dest"

	// Send a RUNNING status first, then a COMPLETED
	if statusCount < 2 {
		resp.State = pb.DataMovementStatusResponse_RUNNING
		resp.CommandStatus.Command = cmd
		resp.CommandStatus.Progress = 50
		resp.CommandStatus.ElapsedTime = (3*time.Second + 139*time.Millisecond).String()
		resp.CommandStatus.LastMessage = "Copied 5.000 GiB (50%) in 3.139 secs (1.480 GiB/s) 1 secs left ..."
		resp.CommandStatus.LastMessageTime = time.Now().Local().String()
		statusCount += 1
	} else {
		resp.State = pb.DataMovementStatusResponse_COMPLETED
		resp.Status = pb.DataMovementStatusResponse_SUCCESS
		resp.Message = fmt.Sprintf("Request %s completed successfully", req.Uid)
		resp.CommandStatus.Command = cmd
		resp.CommandStatus.Progress = 100
		resp.CommandStatus.ElapsedTime = (6*time.Second + 755*time.Millisecond).String()
		resp.CommandStatus.LastMessage = "Copied 10.000 GiB (100%) in 6.755 secs (1.480 GiB/s) done"
		resp.CommandStatus.LastMessageTime = time.Now().String()
		statusCount = 1
	}

	return &resp, nil
}

func (s *simulatedServer) Delete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.DataMovementDeleteResponse{
			Status:  pb.DataMovementDeleteResponse_INVALID,
			Message: fmt.Sprintf("Request %s is invalid", req.Uid),
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.DataMovementDeleteResponse{
			Status:  pb.DataMovementDeleteResponse_NOT_FOUND,
			Message: fmt.Sprintf("Request %s not found", req.Uid),
		}, nil
	}

	delete(s.requests, uid)

	return &pb.DataMovementDeleteResponse{
		Status:  pb.DataMovementDeleteResponse_SUCCESS,
		Message: fmt.Sprintf("Request %s deleted successfully", req.Uid),
	}, nil
}

func (s *simulatedServer) List(ctx context.Context, req *pb.DataMovementListRequest) (*pb.DataMovementListResponse, error) {
	if len(s.requests) > 0 {
		// Get the uuids from the requests map
		uids := make([]string, 0)
		for key := range s.requests {
			uids = append(uids, key.String())
		}

		return &pb.DataMovementListResponse{Uids: uids}, nil
	} else {
		return &pb.DataMovementListResponse{Uids: nil}, nil
	}
}

func (s *simulatedServer) Cancel(ctx context.Context, req *pb.DataMovementCancelRequest) (*pb.DataMovementCancelResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.DataMovementCancelResponse{
			Status:  pb.DataMovementCancelResponse_INVALID,
			Message: fmt.Sprintf("Cancel Request %s is invalid", req.Uid),
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.DataMovementCancelResponse{
			Status:  pb.DataMovementCancelResponse_NOT_FOUND,
			Message: fmt.Sprintf("Request %s not found", uid),
		}, nil
	}

	return &pb.DataMovementCancelResponse{
		Status:  pb.DataMovementCancelResponse_SUCCESS,
		Message: fmt.Sprintf("Cancel Request %s initiated successfully", req.Uid),
	}, nil
}
