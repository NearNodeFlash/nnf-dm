/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MemoryMetricsV140CurrentPeriod - The memory metrics since the last system reset or ClearCurrentPeriod action.
type MemoryMetricsV140CurrentPeriod struct {

	// The number of blocks read since reset.
	BlocksRead int64 `json:"BlocksRead,omitempty"`

	// The number of blocks written since reset.
	BlocksWritten int64 `json:"BlocksWritten,omitempty"`

	// The number of the correctable errors since reset.
	CorrectableECCErrorCount int64 `json:"CorrectableECCErrorCount,omitempty"`

	// The number of the uncorrectable errors since reset.
	UncorrectableECCErrorCount int64 `json:"UncorrectableECCErrorCount,omitempty"`
}
