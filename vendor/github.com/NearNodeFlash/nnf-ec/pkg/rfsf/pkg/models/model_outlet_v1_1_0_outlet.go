/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// OutletV110Outlet - The Outlet schema contains definition for an electrical outlet.
type OutletV110Outlet struct {

	// The OData description of a payload.
	OdataContext string `json:"@odata.context,omitempty"`

	// The current ETag of the resource.
	OdataEtag string `json:"@odata.etag,omitempty"`

	// The unique identifier for a resource.
	OdataId string `json:"@odata.id"`

	// The type of a resource.
	OdataType string `json:"@odata.type"`

	Actions OutletV110Actions `json:"Actions,omitempty"`

	CurrentAmps SensorSensorCurrentExcerpt `json:"CurrentAmps,omitempty"`

	// The description of this resource.  Used for commonality in the schema definitions.
	Description string `json:"Description,omitempty"`

	ElectricalContext SensorElectricalContext `json:"ElectricalContext,omitempty"`

	EnergykWh SensorSensorEnergykWhExcerpt `json:"EnergykWh,omitempty"`

	FrequencyHz SensorSensorExcerpt `json:"FrequencyHz,omitempty"`

	// The identifier that uniquely identifies the resource within the collection of similar resources.
	Id string `json:"Id"`

	IndicatorLED ResourceIndicatorLED `json:"IndicatorLED,omitempty"`

	Links OutletV110Links `json:"Links,omitempty"`

	// An indicator allowing an operator to physically locate this resource.
	LocationIndicatorActive bool `json:"LocationIndicatorActive,omitempty"`

	// The name of the resource or array member.
	Name string `json:"Name"`

	NominalVoltage CircuitNominalVoltageType `json:"NominalVoltage,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	OutletType OutletReceptacleType `json:"OutletType,omitempty"`

	PhaseWiringType CircuitPhaseWiringType `json:"PhaseWiringType,omitempty"`

	PolyPhaseCurrentAmps OutletV110CurrentSensors `json:"PolyPhaseCurrentAmps,omitempty"`

	PolyPhaseVoltage OutletV110VoltageSensors `json:"PolyPhaseVoltage,omitempty"`

	// The number of seconds to delay power on after a PowerControl action to cycle power.  Zero seconds indicates no delay.
	PowerCycleDelaySeconds *float32 `json:"PowerCycleDelaySeconds,omitempty"`

	// Indicates if the outlet can be powered.
	PowerEnabled bool `json:"PowerEnabled,omitempty"`

	// The number of seconds to delay power off after a PowerControl action.  Zero seconds indicates no delay to power off.
	PowerOffDelaySeconds *float32 `json:"PowerOffDelaySeconds,omitempty"`

	// The number of seconds to delay power up after a power cycle or a PowerControl action.  Zero seconds indicates no delay to power up.
	PowerOnDelaySeconds *float32 `json:"PowerOnDelaySeconds,omitempty"`

	// The number of seconds to delay power on after power has been restored.  Zero seconds indicates no delay.
	PowerRestoreDelaySeconds *float32 `json:"PowerRestoreDelaySeconds,omitempty"`

	PowerRestorePolicy CircuitPowerRestorePolicyTypes `json:"PowerRestorePolicy,omitempty"`

	PowerState ResourcePowerState `json:"PowerState,omitempty"`

	PowerWatts SensorSensorPowerExcerpt `json:"PowerWatts,omitempty"`

	// The rated maximum current allowed for this outlet.
	RatedCurrentAmps *float32 `json:"RatedCurrentAmps,omitempty"`

	Status ResourceStatus `json:"Status,omitempty"`

	Voltage SensorSensorVoltageExcerpt `json:"Voltage,omitempty"`

	VoltageType OutletV110VoltageType `json:"VoltageType,omitempty"`
}