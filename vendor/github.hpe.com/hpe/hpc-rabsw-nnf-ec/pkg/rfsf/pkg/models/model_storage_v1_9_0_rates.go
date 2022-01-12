/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// StorageV190Rates - This type describes the various controller rates used for processes such as volume rebuild or consistency checks.
type StorageV190Rates struct {

	// The percentage of controller resources used for performing a data consistency check on volumes.
	ConsistencyCheckRatePercent int64 `json:"ConsistencyCheckRatePercent,omitempty"`

	// The percentage of controller resources used for rebuilding/repairing volumes.
	RebuildRatePercent int64 `json:"RebuildRatePercent,omitempty"`

	// The percentage of controller resources used for transforming volumes from one configuration to another.
	TransformationRatePercent int64 `json:"TransformationRatePercent,omitempty"`
}
