/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ResourceV130Location - The location of a resource.
type ResourceV130Location struct {

	// This indicates the location of the resource.
	Info string `json:"Info,omitempty"`

	// This represents the format of the Info property.
	InfoFormat string `json:"InfoFormat,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	Placement ResourceV130Placement `json:"Placement,omitempty"`

	PostalAddress ResourceV130PostalAddress `json:"PostalAddress,omitempty"`
}
