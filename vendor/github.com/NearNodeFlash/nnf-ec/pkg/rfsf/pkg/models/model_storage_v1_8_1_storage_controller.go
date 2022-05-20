/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// StorageV181StorageController - The StorageController schema describes a storage controller and its properties.  A storage controller represents a physical or virtual storage device that produces volumes.
type StorageV181StorageController struct {

	// The unique identifier for a resource.
	OdataId string `json:"@odata.id"`

	Actions StorageV181StorageControllerActions `json:"Actions,omitempty"`

	Assembly OdataV4IdRef `json:"Assembly,omitempty"`

	// The user-assigned asset tag for this storage controller.
	AssetTag string `json:"AssetTag,omitempty"`

	CacheSummary StorageV181CacheSummary `json:"CacheSummary,omitempty"`

	ControllerRates StorageV181Rates `json:"ControllerRates,omitempty"`

	// The firmware version of this storage controller.
	FirmwareVersion string `json:"FirmwareVersion,omitempty"`

	// The durable names for the storage controller.
	Identifiers []ResourceIdentifier `json:"Identifiers,omitempty"`

	Links StorageV181StorageControllerLinks `json:"Links,omitempty"`

	Location ResourceLocation `json:"Location,omitempty"`

	// The manufacturer of this storage controller.
	Manufacturer string `json:"Manufacturer,omitempty"`

	// The identifier for the member within the collection.
	MemberId string `json:"MemberId"`

	// The model number for the storage controller.
	Model string `json:"Model,omitempty"`

	// The name of the storage controller.
	Name string `json:"Name,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	PCIeInterface PcIeDevicePcIeInterface `json:"PCIeInterface,omitempty"`

	// The part number for this storage controller.
	PartNumber string `json:"PartNumber,omitempty"`

	Ports OdataV4IdRef `json:"Ports,omitempty"`

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

	// The set of RAID types supported by the storage controller.
	SupportedRAIDTypes []VolumeRAIDType `json:"SupportedRAIDTypes,omitempty"`
}