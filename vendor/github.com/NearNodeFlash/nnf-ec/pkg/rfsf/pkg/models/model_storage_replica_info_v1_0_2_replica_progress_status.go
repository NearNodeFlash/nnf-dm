/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi
// StorageReplicaInfoV102ReplicaProgressStatus : Values of ReplicaProgressStatus describe the status of the session with respect to Replication activity.
type StorageReplicaInfoV102ReplicaProgressStatus string

// List of StorageReplicaInfo_v1_0_2_ReplicaProgressStatus
const (
	COMPLETED_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Completed"
	DORMANT_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Dormant"
	INITIALIZING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Initializing"
	PREPARING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Preparing"
	SYNCHRONIZING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Synchronizing"
	RESYNCING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Resyncing"
	RESTORING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Restoring"
	FRACTURING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Fracturing"
	SPLITTING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Splitting"
	FAILING_OVER_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "FailingOver"
	FAILING_BACK_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "FailingBack"
	DETACHING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Detaching"
	ABORTING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Aborting"
	MIXED_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Mixed"
	SUSPENDING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Suspending"
	REQUIRES_FRACTURE_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresFracture"
	REQUIRES_RESYNC_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresResync"
	REQUIRES_ACTIVATE_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresActivate"
	PENDING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Pending"
	REQUIRES_DETACH_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresDetach"
	TERMINATING_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "Terminating"
	REQUIRES_SPLIT_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresSplit"
	REQUIRES_RESUME_SRIV102RPS StorageReplicaInfoV102ReplicaProgressStatus = "RequiresResume"
)
