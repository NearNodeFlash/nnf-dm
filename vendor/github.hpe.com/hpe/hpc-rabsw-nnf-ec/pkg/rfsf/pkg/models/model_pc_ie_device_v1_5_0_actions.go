/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// PcIeDeviceV150Actions - The available actions for this Resource.
type PcIeDeviceV150Actions struct {

	// The available OEM-specific actions for this Resource.
	Oem map[string]interface{} `json:"Oem,omitempty"`
}
