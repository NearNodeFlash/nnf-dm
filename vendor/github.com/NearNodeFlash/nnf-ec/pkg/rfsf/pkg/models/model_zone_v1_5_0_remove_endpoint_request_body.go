/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ZoneV150RemoveEndpointRequestBody - This action removes an endpoint from a zone.
type ZoneV150RemoveEndpointRequestBody struct {

	Endpoint OdataV4IdRef `json:"Endpoint"`

	// The current ETag of the endpoint to remove from the system.
	EndpointETag string `json:"EndpointETag,omitempty"`

	// The current ETag of the zone.
	ZoneETag string `json:"ZoneETag,omitempty"`
}