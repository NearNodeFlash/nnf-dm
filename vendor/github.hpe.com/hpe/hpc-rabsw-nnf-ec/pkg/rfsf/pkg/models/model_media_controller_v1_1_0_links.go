/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MediaControllerV110Links - The links to other resources that are related to this resource.
type MediaControllerV110Links struct {

	// An array of links to the endpoints that connect to this media controller.
	Endpoints []OdataV4IdRef `json:"Endpoints,omitempty"`

	// The number of items in a collection.
	EndpointsodataCount int64 `json:"Endpoints@odata.count,omitempty"`

	// An array of links to the memory domains associated with this media controller.
	MemoryDomains []OdataV4IdRef `json:"MemoryDomains,omitempty"`

	// The number of items in a collection.
	MemoryDomainsodataCount int64 `json:"MemoryDomains@odata.count,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`
}
