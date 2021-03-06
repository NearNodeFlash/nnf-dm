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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NnfDataMovementSpec defines the desired state of DataMovement
type NnfDataMovementSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Source describes the source of the data movement operation
	Source *NnfDataMovementSpecSourceDestination `json:"source,omitempty"`

	// Destination describes the destination of the data movement operation
	Destination *NnfDataMovementSpecSourceDestination `json:"destination,omitempty"`

	// User Id specifies the user ID for the data movement operation. This value is used
	// in conjunction with the group ID to ensure the user has valid permissions to perform
	// the data movement operation.
	UserId uint32 `json:"userId,omitempty"`

	// Group Id specifies the group ID for the data movement operation. This value is used
	// in conjunction with the user ID to ensure the user has valid permissions to perform
	// the data movement operation.
	GroupId uint32 `json:"groupId,omitempty"`

	// Monitor refers to the monitoring state of the resource. When "true", the data movement
	// resource is placed in a mode where it monitors subresources continuously. When "false"
	// a data movement resource is allowed to finish once all subresources finish.
	// +kubebuilder:default:=false
	Monitor bool `json:"monitor,omitempty"`
}

// DataMovementSpecSourceDestination defines the desired source or destination of data movement
type NnfDataMovementSpecSourceDestination struct {

	// Path describes the location of the user data relative to the storage instance
	Path string `json:"path,omitempty"`

	// Storage describes the storage backing this data movement specification; Storage can reference
	// either NNF storage or global Lustre storage depending on the object references Kind field.
	Storage *corev1.ObjectReference `json:"storageInstance,omitempty"`

	// Access references the NNF Access element that is needed to perform data movement. This provides
	// details as to the mount path and backing storage across NNF Nodes.
	Access *corev1.ObjectReference `json:"access,omitempty"`
}

// DataMovementStatus defines the observed state of DataMovement
type NnfDataMovementStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Node Status reflects the status of individual NNF Node Data Movement operations
	NodeStatus []NnfDataMovementNodeStatus `json:"nodeStatus,omitempty"`

	// Conditions represents an array of conditions that refect the current
	// status of the data movement operation. Each condition type must be
	// one of Starting, Running, or Finished, reflect the three states that
	// data movement performs.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// StartTime reflects the time at which the Data Movement operation started.
	StartTime *metav1.MicroTime `json:"startTime,omitempty"`

	// EndTime reflects the time at which the Data Movement operation ended.
	EndTime *metav1.MicroTime `json:"endTime,omitempty"`
}

// NnfDataMovementNodeStatus defines the observed state of a DataMovementNode
type NnfDataMovementNodeStatus struct {
	// Node is the node who's status this status element describes
	Node string `json:"node,omitempty"`

	// Count is the total number of resources managed by this node
	Count uint32 `json:"count,omitempty"`

	// Running is the number of resources running under this node
	Running uint32 `json:"running,omitemtpy"`

	// Complete is the number of resource completed by this node
	Complete uint32 `json:"complete,omitempty"`

	// Status is an array of status strings reported by resources in this node
	Status []string `json:"status,omitempty"`

	// Messages is an array of error messages reported by resources in this node
	Messages []string `json:"messages,omitempty"`
}

// Types describing the various data movement status conditions.
const (
	DataMovementConditionTypeStarting = "Starting"
	DataMovementConditionTypeRunning  = "Running"
	DataMovementConditionTypeFinished = "Finished"
)

// Reasons describing the various data movement status conditions. Must be
// in CamelCase format (see metav1.Condition)
const (
	DataMovementConditionReasonSuccess = "Success"
	DataMovementConditionReasonFailed  = "Failed"
	DataMovementConditionReasonInvalid = "Invalid"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NnfDataMovement is the Schema for the datamovements API
type NnfDataMovement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NnfDataMovementSpec   `json:"spec,omitempty"`
	Status NnfDataMovementStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NnfDataMovementList contains a list of NnfDataMovement
type NnfDataMovementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NnfDataMovement `json:"items"`
}

func (n *NnfDataMovementList) GetObjectList() []client.Object {
	objectList := []client.Object{}

	for i := range n.Items {
		objectList = append(objectList, &n.Items[i])
	}

	return objectList
}

func init() {
	SchemeBuilder.Register(&NnfDataMovement{}, &NnfDataMovementList{})
}
