/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MemoryV1100PowerManagementPolicy - Power management policy information.
type MemoryV1100PowerManagementPolicy struct {

	// Average power budget, in milliwatts.
	AveragePowerBudgetMilliWatts int64 `json:"AveragePowerBudgetMilliWatts,omitempty"`

	// Maximum TDP in milliwatts.
	MaxTDPMilliWatts int64 `json:"MaxTDPMilliWatts,omitempty"`

	// Peak power budget, in milliwatts.
	PeakPowerBudgetMilliWatts int64 `json:"PeakPowerBudgetMilliWatts,omitempty"`

	// An indication of whether the power management policy is enabled.
	PolicyEnabled bool `json:"PolicyEnabled,omitempty"`
}
