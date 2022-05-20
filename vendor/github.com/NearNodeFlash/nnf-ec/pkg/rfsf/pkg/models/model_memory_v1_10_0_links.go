/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MemoryV1100Links - The links to other resources that are related to this resource.
type MemoryV1100Links struct {

	Chassis OdataV4IdRef `json:"Chassis,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`
}