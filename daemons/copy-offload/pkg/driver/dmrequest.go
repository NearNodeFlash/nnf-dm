/*
 * Copyright 2024-2025 Hewlett Packard Enterprise Development LP
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

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// DMRequest represents the content of one http request. This has a
// one-to-one relationship with a DriverRequest object.
// This is the v1.0 apiVersion. See COPY_OFFLOAD_API_VERSION.
type DMRequest struct {
	ComputeName string `json:"computeName"`

	// The name and namespace of the initiating workflow
	WorkflowName      string `json:"workflowName"`
	WorkflowNamespace string `json:"workflowNamespace"`
	// Source file or directory
	SourcePath string `json:"sourcePath"`
	// Destination file or directory
	DestinationPath string `json:"destinationPath"`
	// If True, the data movement command runs `/bin/true` rather than perform actual data movement
	Dryrun bool `json:"dryrun"`
	// Extra options to pass to `dcp` if present in the Data Movement command.
	DcpOptions string `json:"dcpOptions"`
	// If true, enable server-side logging of stdout when the command is successful. Failure output is always logged.
	LogStdout bool `json:"logStdout"`
	// If true, store stdout in DataMovementStatusResponse.Message when the command is successful. Failure output is always contained in the message.
	StoreStdout bool `json:"storeStdout"`
	// The number of slots specified in the MPI hostfile. A value of 0 disables the use of slots in
	// the hostfile. -1 will defer to the server side configuration.
	Slots int32 `json:"slots"`
	// The number of max_slots specified in the MPI hostfile. A value of 0 disables the use of
	// max_slots in the hostfile. -1 will defer to the server side configuration.
	MaxSlots int32 `json:"maxSlots"`
	// The name of the data movement configuration profile to use. The above parameters (e.g. slots,
	// logStdout) will override the settings defined by the profile. This profile must exist on the
	// server otherwise the data movement operation will be invalid. Empty will default to the
	// default profile.
	DMProfile string `json:"dmProfile"`
	// Extra options to pass to `mpirun` if present in the Data Movement command.
	MpirunOptions string `json:"mpirunOptions"`
}

// StatusRequest represents the content of one http request. This has a
// one-to-one relationship with a DriverRequest object.
// This is the v1.0 apiVersion. See COPY_OFFLOAD_API_VERSION.
type StatusRequest struct {
	// The name and namespace of the initiating workflow
	WorkflowName      string `json:"workflowName"`
	WorkflowNamespace string `json:"workflowNamespace"`
	// Name of the copy request.
	RequestName string `json:"requestName"`
	// Max number of seconds to wait for the completion of the copy request.
	// A negative number results in an indefinite wait.
	MaxWaitSecs int `json:"maxWaitSecs"`
}

func (m *DMRequest) Validator() error {

	if m.ComputeName == "" {
		return fmt.Errorf("compute name must be supplied")
	}
	if m.WorkflowName == "" {
		return fmt.Errorf("workflow name must be supplied")
	}
	if m.SourcePath == "" {
		return fmt.Errorf("source path must be supplied")
	}
	if m.DestinationPath == "" {
		return fmt.Errorf("destination path must be supplied")
	}
	if m.WorkflowNamespace == "" {
		m.WorkflowNamespace = corev1.NamespaceDefault
	}
	if m.Slots < -1 {
		return fmt.Errorf("slots must be -1 (defer to profile), 0 (disable), or a positive integer")
	}
	if m.MaxSlots < -1 {
		return fmt.Errorf("maxSlots must be -1 (defer to profile), 0 (disable), or a positive integer")
	}

	return nil
}

func (m *StatusRequest) Validator() error {

	if m.RequestName == "" {
		return fmt.Errorf("request name must be supplied")
	}
	if m.WorkflowName == "" {
		return fmt.Errorf("workflow name must be supplied")
	}
	if m.WorkflowNamespace == "" {
		m.WorkflowNamespace = corev1.NamespaceDefault
	}

	return nil
}
