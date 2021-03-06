/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi
// ResourceV157LocationType : The location types for PartLocation.
type ResourceV157LocationType string

// List of Resource_v1_5_7_LocationType
const (
	SLOT_RV157LT ResourceV157LocationType = "Slot"
	BAY_RV157LT ResourceV157LocationType = "Bay"
	CONNECTOR_RV157LT ResourceV157LocationType = "Connector"
	SOCKET_RV157LT ResourceV157LocationType = "Socket"
)
