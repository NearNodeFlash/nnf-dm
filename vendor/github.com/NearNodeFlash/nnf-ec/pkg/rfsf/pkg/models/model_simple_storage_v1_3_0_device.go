/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// SimpleStorageV130Device - A storage device, such as a disk drive or optical media device.
type SimpleStorageV130Device struct {

	// The size, in bytes, of the storage device.
	CapacityBytes int64 `json:"CapacityBytes,omitempty"`

	// The name of the manufacturer of this device.
	Manufacturer string `json:"Manufacturer,omitempty"`

	// The product model number of this device.
	Model string `json:"Model,omitempty"`

	// The name of the Resource or array member.
	Name string `json:"Name"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	Status ResourceStatus `json:"Status,omitempty"`
}
