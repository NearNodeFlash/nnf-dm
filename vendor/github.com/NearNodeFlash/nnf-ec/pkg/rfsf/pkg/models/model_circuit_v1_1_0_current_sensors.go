/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// CircuitV110CurrentSensors - The current sensors for this circuit.
type CircuitV110CurrentSensors struct {

	Line1 SensorSensorCurrentExcerpt `json:"Line1,omitempty"`

	Line2 SensorSensorCurrentExcerpt `json:"Line2,omitempty"`

	Line3 SensorSensorCurrentExcerpt `json:"Line3,omitempty"`

	Neutral SensorSensorCurrentExcerpt `json:"Neutral,omitempty"`
}