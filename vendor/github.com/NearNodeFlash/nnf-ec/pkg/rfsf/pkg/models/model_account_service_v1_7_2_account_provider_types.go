/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type AccountServiceV172AccountProviderTypes string

// List of AccountService_v1_7_2_AccountProviderTypes
const (
	REDFISH_SERVICE_ASV172APT AccountServiceV172AccountProviderTypes = "RedfishService"
	ACTIVE_DIRECTORY_SERVICE_ASV172APT AccountServiceV172AccountProviderTypes = "ActiveDirectoryService"
	LDAP_SERVICE_ASV172APT AccountServiceV172AccountProviderTypes = "LDAPService"
	OEM_ASV172APT AccountServiceV172AccountProviderTypes = "OEM"
)
