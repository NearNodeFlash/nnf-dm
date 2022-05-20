/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// FabricAdapterV100FabricAdapter - A FabricAdapter represents the physical fabric adapter capable of connecting to an interconnect fabric.  Examples include but are not limited to Ethernet, NVMe over Fabrics, Gen-Z, and SAS fabric adapters.
type FabricAdapterV100FabricAdapter struct {

	// The OData description of a payload.
	OdataContext string `json:"@odata.context,omitempty"`

	// The current ETag of the resource.
	OdataEtag string `json:"@odata.etag,omitempty"`

	// The unique identifier for a resource.
	OdataId string `json:"@odata.id"`

	// The type of a resource.
	OdataType string `json:"@odata.type"`

	// The manufacturer name for the ASIC of this fabric adapter.
	ASICManufacturer string `json:"ASICManufacturer,omitempty"`

	// The part number for the ASIC on this fabric adapter.
	ASICPartNumber string `json:"ASICPartNumber,omitempty"`

	// The revision identifier for the ASIC on this fabric adapter.
	ASICRevisionIdentifier string `json:"ASICRevisionIdentifier,omitempty"`

	Actions FabricAdapterV100Actions `json:"Actions,omitempty"`

	// The description of this resource.  Used for commonality in the schema definitions.
	Description string `json:"Description,omitempty"`

	// The firmware version of this fabric adapter.
	FirmwareVersion string `json:"FirmwareVersion,omitempty"`

	GenZ FabricAdapterV100GenZ `json:"GenZ,omitempty"`

	// The identifier that uniquely identifies the resource within the collection of similar resources.
	Id string `json:"Id"`

	Links FabricAdapterV100Links `json:"Links,omitempty"`

	// The manufacturer or OEM of this fabric adapter.
	Manufacturer string `json:"Manufacturer,omitempty"`

	// The model string for this fabric adapter.
	Model string `json:"Model,omitempty"`

	// The name of the resource or array member.
	Name string `json:"Name"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	PCIeInterface PcIeDevicePcIeInterface `json:"PCIeInterface,omitempty"`

	// The part number for this fabric adapter.
	PartNumber string `json:"PartNumber,omitempty"`

	Ports OdataV4IdRef `json:"Ports,omitempty"`

	// The manufacturer SKU for this fabric adapter.
	SKU string `json:"SKU,omitempty"`

	// The serial number for this fabric adapter.
	SerialNumber string `json:"SerialNumber,omitempty"`

	// The spare part number for this fabric adapter.
	SparePartNumber string `json:"SparePartNumber,omitempty"`

	Status ResourceStatus `json:"Status,omitempty"`

	UUID string `json:"UUID,omitempty"`
}