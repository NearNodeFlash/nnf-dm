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

func (s *simulatedServer) Status(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.DataMovementStatusResponse{
			State:   pb.DataMovementStatusResponse_UNKNOWN_STATE,
			Status:  pb.DataMovementStatusResponse_INVALID,
			Message: fmt.Sprintf("Request %s is invalid", req.Uid),
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.DataMovementStatusResponse{
			State:   pb.DataMovementStatusResponse_UNKNOWN_STATE,
			Status:  pb.DataMovementStatusResponse_NOT_FOUND,
			Message: fmt.Sprintf("Request %s not found", req.Uid),
		}, nil
	}

	return &pb.DataMovementStatusResponse{
		State:   pb.DataMovementStatusResponse_COMPLETED,
		Status:  pb.DataMovementStatusResponse_SUCCESS,
		Message: fmt.Sprintf("Request %s completed successfully", req.Uid),
	}, nil
}

func (s *simulatedServer) Delete(ctx context.Context, req *pb.DataMovementDeleteRequest) (*pb.DataMovementDeleteResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.DataMovementDeleteResponse{
			Status: pb.DataMovementDeleteResponse_INVALID,
		}, nil
	}

	if _, ok := s.requests[uid]; !ok {
		return &pb.DataMovementDeleteResponse{
			Status: pb.DataMovementDeleteResponse_NOT_FOUND,
		}, nil
	}

	delete(s.requests, uid)

	return &pb.DataMovementDeleteResponse{
		Status: pb.DataMovementDeleteResponse_DELETED,
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
