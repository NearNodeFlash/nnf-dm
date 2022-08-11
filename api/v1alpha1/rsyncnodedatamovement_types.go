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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RsyncNodeDataMovementSpec defines the desired state of RsyncNodeDataMovement
type RsyncNodeDataMovementSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Initiator is used to define the compute resource that initiated the data movement request. This value must match
	// one of the compute resources selected by the WLM to run the job, or be empty if this is not a compute initiated
	// data transfer.
	Initiator string `json:"initiator,omitempty"`

	// Source file or directory, the contents of which are copied to the destination
	Source string `json:"source,omitempty"`

	// Destination file or directory, the receipent of the source contents
	Destination string `json:"destination,omitempty"`

	// UserId is the user ID of a compute initated data movement. This value is used to ensure correct permissions
	// are granted to the initiator.
	UserId uint32 `json:"userId,omitempty"`

	// GroupId is the group ID of a compute initiated data movement. This value is used to ensure correct permissions
	// are granted to the initiator.
	GroupId uint32 `json:"groupId,omitempty"`

	// DryRun specifies that this data movement request should show what would have been transferred without actually
	// performing any copy operation.
	DryRun bool `json:"dryRun,omitempty"`

	// Cancel is a flag to indicate that this data movement should be
	// terminated. This value should not be set on creation.
	// +kubebuilder:default:=false
	Cancel bool `json:"cancel,omitempty"`

	// Use an alternative command to simulate data movement (e.g. sleep 120).
	// Empty string performs actual data movement.
	Simulate string `json:"simulate,omitempty"`
}

// RsyncNodeDataMovementStatus defines the observed state of RsyncNodeDataMovement
type RsyncNodeDataMovementStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Start Time of the data movement operation
	StartTime metav1.MicroTime `json:"startTime,omitempty"`

	// End Time of the data movement operation
	EndTime metav1.MicroTime `json:"endTime,omitempty"`

	// Current state of the data movement operation
	State string `json:"state,omitempty"`

	// Current status of the data movement operation; valid only when the state is finished.
	Status string `json:"status,omitempty"`

	// Message provides details on the data movement operation; can be used to diagnose problems pertaining to
	// a failed status.
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status",description="Current status of the data movement operation"
//+kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.state",description="Current state of the data movement operation"
//+kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// RsyncNodeDataMovement is the Schema for the rsyncnodedatamovements API
type RsyncNodeDataMovement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RsyncNodeDataMovementSpec   `json:"spec,omitempty"`
	Status RsyncNodeDataMovementStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RsyncNodeDataMovementList contains a list of RsyncNodeDataMovement
type RsyncNodeDataMovementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RsyncNodeDataMovement `json:"items"`
}

func (r *RsyncNodeDataMovementList) GetObjectList() []client.Object {
	objectList := []client.Object{}

	for i := range r.Items {
		objectList = append(objectList, &r.Items[i])
	}

	return objectList
}

func init() {
	SchemeBuilder.Register(&RsyncNodeDataMovement{}, &RsyncNodeDataMovementList{})
}
