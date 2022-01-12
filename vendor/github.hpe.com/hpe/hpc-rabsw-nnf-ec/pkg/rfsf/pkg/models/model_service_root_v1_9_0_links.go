/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ServiceRootV190Links - The links to other Resources that are related to this Resource.
type ServiceRootV190Links struct {

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	Sessions OdataV4IdRef `json:"Sessions"`
}
