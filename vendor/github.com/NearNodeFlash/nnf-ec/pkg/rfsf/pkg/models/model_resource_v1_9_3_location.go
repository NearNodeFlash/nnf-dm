/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ResourceV193Location - The location of a resource.
type ResourceV193Location struct {

	// The altitude of the resource in meters.
	AltitudeMeters *float32 `json:"AltitudeMeters,omitempty"`

	// An array of contact information.
	Contacts []ResourceV193ContactInfo `json:"Contacts,omitempty"`

	// The location of the resource.
	Info string `json:"Info,omitempty"`

	// The format of the Info property.
	InfoFormat string `json:"InfoFormat,omitempty"`

	// The latitude of the resource.
	Latitude *float32 `json:"Latitude,omitempty"`

	// The longitude of the resource in degrees.
	Longitude *float32 `json:"Longitude,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	PartLocation ResourceV193PartLocation `json:"PartLocation,omitempty"`

	Placement ResourceV193Placement `json:"Placement,omitempty"`

	PostalAddress ResourceV193PostalAddress `json:"PostalAddress,omitempty"`
}
