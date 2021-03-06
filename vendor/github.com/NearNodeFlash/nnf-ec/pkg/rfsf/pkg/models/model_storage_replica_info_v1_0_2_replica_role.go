/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi
// StorageReplicaInfoV102ReplicaRole : Values of ReplicaRole specify whether the resource is a source of replication or the target of replication.
type StorageReplicaInfoV102ReplicaRole string

// List of StorageReplicaInfo_v1_0_2_ReplicaRole
const (
	SOURCE_SRIV102RR StorageReplicaInfoV102ReplicaRole = "Source"
	TARGET_SRIV102RR StorageReplicaInfoV102ReplicaRole = "Target"
)
