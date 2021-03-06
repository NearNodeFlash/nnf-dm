/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ResourceBlockV133ResourceBlock - The ResourceBlock schema contains definitions resource blocks, its components, and affinity to composed devices.
type ResourceBlockV133ResourceBlock struct {

	// The OData description of a payload.
	OdataContext string `json:"@odata.context,omitempty"`

	// The current ETag of the resource.
	OdataEtag string `json:"@odata.etag,omitempty"`

	// The unique identifier for a resource.
	OdataId string `json:"@odata.id"`

	// The type of a resource.
	OdataType string `json:"@odata.type"`

	Actions ResourceBlockV133Actions `json:"Actions,omitempty"`

	CompositionStatus ResourceBlockV133CompositionStatus `json:"CompositionStatus"`

	// An array of links to the computer systems available in this resource block.
	ComputerSystems []OdataV4IdRef `json:"ComputerSystems,omitempty"`

	// The number of items in a collection.
	ComputerSystemsodataCount int64 `json:"ComputerSystems@odata.count,omitempty"`

	// The description of this resource.  Used for commonality in the schema definitions.
	Description string `json:"Description,omitempty"`

	// An array of links to the drives available in this resource block.
	Drives []OdataV4IdRef `json:"Drives,omitempty"`

	// The number of items in a collection.
	DrivesodataCount int64 `json:"Drives@odata.count,omitempty"`

	// An array of links to the Ethernet interfaces available in this resource block.
	EthernetInterfaces []OdataV4IdRef `json:"EthernetInterfaces,omitempty"`

	// The number of items in a collection.
	EthernetInterfacesodataCount int64 `json:"EthernetInterfaces@odata.count,omitempty"`

	// The identifier that uniquely identifies the resource within the collection of similar resources.
	Id string `json:"Id"`

	Links ResourceBlockV133Links `json:"Links,omitempty"`

	// An array of links to the memory available in this resource block.
	Memory []OdataV4IdRef `json:"Memory,omitempty"`

	// The number of items in a collection.
	MemoryodataCount int64 `json:"Memory@odata.count,omitempty"`

	// The name of the resource or array member.
	Name string `json:"Name"`

	// An array of links to the Network Interfaces available in this resource block.
	NetworkInterfaces []OdataV4IdRef `json:"NetworkInterfaces,omitempty"`

	// The number of items in a collection.
	NetworkInterfacesodataCount int64 `json:"NetworkInterfaces@odata.count,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	// An array of links to the processors available in this resource block.
	Processors []OdataV4IdRef `json:"Processors,omitempty"`

	// The number of items in a collection.
	ProcessorsodataCount int64 `json:"Processors@odata.count,omitempty"`

	// The types of resources available on this resource block.
	ResourceBlockType []ResourceBlockV133ResourceBlockType `json:"ResourceBlockType"`

	// An array of links to the simple storage available in this resource block.
	SimpleStorage []OdataV4IdRef `json:"SimpleStorage,omitempty"`

	// The number of items in a collection.
	SimpleStorageodataCount int64 `json:"SimpleStorage@odata.count,omitempty"`

	Status ResourceStatus `json:"Status,omitempty"`

	// An array of links to the storage available in this resource block.
	Storage []OdataV4IdRef `json:"Storage,omitempty"`

	// The number of items in a collection.
	StorageodataCount int64 `json:"Storage@odata.count,omitempty"`
}
