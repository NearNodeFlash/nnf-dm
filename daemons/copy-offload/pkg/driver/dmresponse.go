/*
 * Copyright 2025 Hewlett Packard Enterprise Development LP
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

package driver

import "fmt"

type DataMovementStatusResponse_State string

const (
	DataMovementStatusResponse_PENDING       DataMovementStatusResponse_State = "pending"       // The request is created but has a pending status
	DataMovementStatusResponse_STARTING      DataMovementStatusResponse_State = "starting"      // The request was created and is in the act of starting
	DataMovementStatusResponse_RUNNING       DataMovementStatusResponse_State = "running"       // The data movement request is actively running
	DataMovementStatusResponse_COMPLETED     DataMovementStatusResponse_State = "completed"     // The data movement request has completed
	DataMovementStatusResponse_CANCELLING    DataMovementStatusResponse_State = "cancelling"    // The data movement request has started the cancellation process
	DataMovementStatusResponse_UNKNOWN_STATE DataMovementStatusResponse_State = "unknown state" // Unknown state
)

type DataMovementStatusResponse_Status string

const (
	DataMovementStatusResponse_INVALID        DataMovementStatusResponse_Status = "invalid"        // The request was found to be invalid. See `message` for details
	DataMovementStatusResponse_NOT_FOUND      DataMovementStatusResponse_Status = "not found"      // The request with the supplied UID was not found
	DataMovementStatusResponse_SUCCESS        DataMovementStatusResponse_Status = "success"        // The request completed with success
	DataMovementStatusResponse_FAILED         DataMovementStatusResponse_Status = "failed"         // The request failed. See `message` for details
	DataMovementStatusResponse_CANCELLED      DataMovementStatusResponse_Status = "cancelled"      // The request was cancelled
	DataMovementStatusResponse_UNKNOWN_STATUS DataMovementStatusResponse_Status = "unknown status" // Unknown status
)

// Data movement operations contain a CommandStatus in the Status object. This information relays the current state of the data movement progress.
type DataMovementCommandStatus struct {
	// Command used to perform Data Movement
	Command string `json:"command,omitempty"`
	// Progress percentage (0-100%) of the data movement based on the output of `dcp`. `dcp` must be present in the command.
	Progress int32 `json:"progress,omitempty"`
	// Duration of how long the operation has been running (e.g. 1m30s)
	ElapsedTime string `json:"elapsedTime,omitempty"`
	// The most recent line of output from the data movement command
	LastMessage string `json:"lastMessage,omitempty"`
	// The time (local) of lastMessage
	LastMessageTime string `json:"lastMessageTime,omitempty"`
}

// The Data Movement Status Response message defines the current status of a data movement request.
// This is the v1.0 apiVersion. See COPY_OFFLOAD_API_VERSION.
type DataMovementStatusResponse_v1_0 struct {
	// Current state of the Data Movement Request
	State DataMovementStatusResponse_State `json:"state,omitempty"`
	// Current status of the Data Movement Request
	Status DataMovementStatusResponse_Status `json:"status,omitempty"`
	// Any extra information supplied with the state/status
	Message string `json:"message,omitempty"`
	// Current state/progress of the Data Movement command
	CommandStatus *DataMovementCommandStatus `json:"commandStatus,omitempty"`
	// The start time (local) of the data movement operation
	StartTime string `json:"startTime,omitempty"`
	// The end time (local) of the data movement operation
	EndTime string `json:"endTime,omitempty"`
}

func (dmr *DataMovementStatusResponse_v1_0) String() error {
	if dmr.State == "" {
		return fmt.Errorf("State must be set")
	}
	if dmr.Status == "" {
		return fmt.Errorf("Status must be set")
	}
	if dmr.CommandStatus == nil {
		return fmt.Errorf("CommandStatus must be set")
	}
	if dmr.StartTime == "" {
		return fmt.Errorf("StartTime must be set")
	}
	if dmr.EndTime == "" {
		return fmt.Errorf("EndTime must be set")
	}
	return nil
}
