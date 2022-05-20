/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// StorageV190Actions - The available actions for this resource.
type StorageV190Actions struct {

	StorageSetEncryptionKey StorageV190SetEncryptionKey `json:"#Storage.SetEncryptionKey,omitempty"`

	// The available OEM-specific actions for this resource.
	Oem map[string]interface{} `json:"Oem,omitempty"`
}