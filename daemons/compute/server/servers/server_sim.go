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
	"sync"
	"time"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"
	"github.com/NearNodeFlash/nnf-dm/daemons/compute/server/version"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// For a StatusResponse, how many times should we return RUNNING before COMPLETE
	statusResponseRunningCount = 1

	// For a StatusResponse, how many times should we return CANCELLING before COMPLETE
	statusResponseCancellingCount = 1
)

// Simulated server implements a simple Data Mover Server that accepts and completes all data movemement requests.
type simulatedServer struct {
	pb.UnimplementedDataMoverServer

	requests      map[uuid.UUID]requestData
	requestsMutex sync.Mutex
}

type requestData struct {
	// Keep track of how many status responses we've sent to facilitate sending RUNNING before COMPLETE
	statusResponseCount int
	cancelResponseCount int
	request             *pb.DataMovementCreateRequest
}

func (s *simulatedServer) addRequest(uid uuid.UUID, r requestData) {
	s.requestsMutex.Lock()
	defer s.requestsMutex.Unlock()

	s.requests[uid] = r
}

func (s *simulatedServer) getRequest(uid uuid.UUID) (requestData, bool) {
	s.requestsMutex.Lock()
	defer s.requestsMutex.Unlock()

	r, ok := s.requests[uid]
	return r, ok
}

func CreateSimulatedServer(opts *ServerOptions) (*simulatedServer, error) {
	return &simulatedServer{requests: make(map[uuid.UUID]requestData)}, nil
}

func (s *simulatedServer) StartManager() error {
	return nil
}

func (*simulatedServer) Version(context.Context, *emptypb.Empty) (*pb.DataMovementVersionResponse, error) {
	return &pb.DataMovementVersionResponse{
		Version:     version.BuildVersion(),
		ApiVersions: version.ApiVersions(),
	}, nil
}

func (s *simulatedServer) Create(ctx context.Context, req *pb.DataMovementCreateRequest) (*pb.DataMovementCreateResponse, error) {
	uid := uuid.New()

	s.addRequest(uid, requestData{statusResponseCount: 0, cancelResponseCount: 0, request: req})

	return &pb.DataMovementCreateResponse{Uid: uid.String()}, nil
}

func (s *simulatedServer) Status(ctx context.Context, req *pb.DataMovementStatusRequest) (*pb.DataMovementStatusResponse, error) {
	uid, err := uuid.Parse(req.Uid)
	if err != nil {
		return &pb.DataMovementStatusResponse{
			State:         pb.DataMovementStatusResponse_UNKNOWN_STATE,
			Status:        pb.DataMovementStatusResponse_INVALID,
			Message:       fmt.Sprintf("Request %s is invalid", req.Uid),
			CommandStatus: nil,
		}, nil
	}

	reqData, ok := s.getRequest(uid)
	if !ok {
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

	now := time.Now().Local()

	// If a request was cancelled, send a CANCELLING response first, then COMPLETE
	if reqData.cancelResponseCount > 0 && reqData.cancelResponseCount <= statusResponseCancellingCount {
		resp.State = pb.DataMovementStatusResponse_CANCELLING
		reqData.cancelResponseCount += 1
	} else if reqData.cancelResponseCount > statusResponseCancellingCount {
		resp.State = pb.DataMovementStatusResponse_COMPLETED
		resp.Status = pb.DataMovementStatusResponse_CANCELLED
		// Otherwise, send RUNNING status first, then COMPLETED
	} else if reqData.statusResponseCount < statusResponseRunningCount {
		resp.State = pb.DataMovementStatusResponse_RUNNING

		if reqData.request.Dryrun {
			resp.CommandStatus.Command = "true"
		} else {
			resp.CommandStatus.Command = cmd
			resp.CommandStatus.Progress = 50
			resp.CommandStatus.LastMessage = "Copied 5.000 GiB (50%) in 3.139 secs (1.480 GiB/s) 1 secs left ..."
			resp.CommandStatus.LastMessageTime = time.Now().Local().String()
			resp.CommandStatus.ElapsedTime = (3*time.Second + 139*time.Millisecond).String()
		}
		resp.StartTime = now.String()
		resp.EndTime = now.Add(30 * time.Second).String()

		reqData.statusResponseCount += 1
	} else {
		resp.State = pb.DataMovementStatusResponse_COMPLETED
		resp.Status = pb.DataMovementStatusResponse_SUCCESS
		resp.Message = fmt.Sprintf("Request %s completed successfully", req.Uid)

		if reqData.request.Dryrun {
			resp.CommandStatus.Command = "true"
		} else {
			resp.CommandStatus.Command = cmd
			resp.CommandStatus.Progress = 100
			resp.CommandStatus.LastMessage = "Copied 10.000 GiB (100%) in 6.755 secs (1.480 GiB/s) done"
			resp.CommandStatus.LastMessageTime = time.Now().String()
			resp.CommandStatus.ElapsedTime = (6*time.Second + 755*time.Millisecond).String()
		}
		resp.StartTime = now.String()
		resp.EndTime = now.Add(30 * time.Second).String()
	}

	s.addRequest(uid, reqData)

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

	reqData, ok := s.requests[uid]
	if !ok {
		return &pb.DataMovementCancelResponse{
			Status:  pb.DataMovementCancelResponse_NOT_FOUND,
			Message: fmt.Sprintf("Request %s not found", uid),
		}, nil
	}

	// Set the count so we can use Status to get cancel statuses
	reqData.cancelResponseCount = 1
	s.addRequest(uid, reqData)

	return &pb.DataMovementCancelResponse{
		Status:  pb.DataMovementCancelResponse_SUCCESS,
		Message: fmt.Sprintf("Cancel Request %s initiated successfully", req.Uid),
	}, nil
}
