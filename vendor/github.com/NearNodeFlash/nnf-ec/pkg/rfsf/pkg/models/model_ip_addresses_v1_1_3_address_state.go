/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type IPAddressesV113AddressState string

// List of IPAddresses_v1_1_3_AddressState
const (
	PREFERRED_IPAV113AST IPAddressesV113AddressState = "Preferred"
	DEPRECATED_IPAV113AST IPAddressesV113AddressState = "Deprecated"
	TENTATIVE_IPAV113AST IPAddressesV113AddressState = "Tentative"
	FAILED_IPAV113AST IPAddressesV113AddressState = "Failed"
)
