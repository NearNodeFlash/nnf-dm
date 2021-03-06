/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// IpAddressesV1010IPv6Address - This type describes an IPv6 address.
type IpAddressesV1010IPv6Address struct {

	// The IPv6 address.
	Address string `json:"Address,omitempty"`

	AddressOrigin IPAddressesV1010IPv6AddressOrigin `json:"AddressOrigin,omitempty"`

	AddressState IPAddressesV1010AddressState `json:"AddressState,omitempty"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	PrefixLength int64 `json:"PrefixLength,omitempty"`
}
