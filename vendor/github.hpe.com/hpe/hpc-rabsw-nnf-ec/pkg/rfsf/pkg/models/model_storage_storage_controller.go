/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// StorageStorageController - The StorageController schema describes a storage controller and its properties.  A storage controller represents a physical or virtual storage device that produces volumes.
type StorageStorageController struct {

	// The unique identifier for a resource.
	OdataId string `json:"@odata.id"`

	// The user-assigned asset tag for this storage controller.
	AssetTag string `json:"AssetTag,omitempty"`

	// The firmware version of this storage controller.
	FirmwareVersion string `json:"FirmwareVersion,omitempty"`

	// The durable names for the storage controller.
	Identifiers []ResourceIdentifier `json:"Identifiers,omitempty"`

	// The manufacturer of this storage controller.
	Manufacturer string `json:"Manufacturer,omitempty"`

	// The identifier for the member within the collection.
	MemberId string `json:"MemberId"`

	// The model number for the storage controller.
	Model string `json:"Model,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	// The part number for this storage controller.
	PartNumber string `json:"PartNumber,omitempty"`

	// The SKU for this storage controller.
	SKU string `json:"SKU,omitempty"`

	// The serial number for this storage controller.
	SerialNumber string `json:"SerialNumber,omitempty"`

	// The maximum speed of the storage controller's device interface.
	SpeedGbps *float32 `json:"SpeedGbps,omitempty"`

	Status ResourceStatus `json:"Status,omitempty"`

	// The supported set of protocols for communicating to this storage controller.
	SupportedControllerProtocols []ProtocolProtocol `json:"SupportedControllerProtocols,omitempty"`

	// The protocols that the storage controller can use to communicate with attached devices.
	SupportedDeviceProtocols []ProtocolProtocol `json:"SupportedDeviceProtocols,omitempty"`

	Links StorageV190StorageControllerLinks `json:"Links,omitempty"`

	Actions StorageV190StorageControllerActions `json:"Actions,omitempty"`

	// The name of the storage controller.
	Name string `json:"Name,omitempty"`

	Assembly OdataV4IdRef `json:"Assembly,omitempty"`

	Location ResourceLocation `json:"Location,omitempty"`

	CacheSummary StorageV190CacheSummary `json:"CacheSummary,omitempty"`

	PCIeInterface PcIeDevicePcIeInterface `json:"PCIeInterface,omitempty"`

	// The set of RAID types supported by the storage controller.
	SupportedRAIDTypes []VolumeRAIDType `json:"SupportedRAIDTypes,omitempty"`

	ControllerRates StorageV190Rates `json:"ControllerRates,omitempty"`

	Ports OdataV4IdRef `json:"Ports,omitempty"`
}
