/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ResourceBlockV133Links - The links to other resources that are related to this resource.
type ResourceBlockV133Links struct {

	// An array of links to the chassis in which this resource block is contained.
	Chassis []OdataV4IdRef `json:"Chassis,omitempty"`

	// The number of items in a collection.
	ChassisodataCount int64 `json:"Chassis@odata.count,omitempty"`

	// An array of links to the computer systems that are composed from this resource block.
	ComputerSystems []OdataV4IdRef `json:"ComputerSystems,omitempty"`

	// The number of items in a collection.
	ComputerSystemsodataCount int64 `json:"ComputerSystems@odata.count,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	// An array of links to the zones in which this resource block is bound.
	Zones []OdataV4IdRef `json:"Zones,omitempty"`

	// The number of items in a collection.
	ZonesodataCount int64 `json:"Zones@odata.count,omitempty"`
}
