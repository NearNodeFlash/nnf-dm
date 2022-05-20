/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type ManagerV1100ManagerType string

// List of Manager_v1_10_0_ManagerType
const (
	MANAGEMENT_CONTROLLER_MV1100MT ManagerV1100ManagerType = "ManagementController"
	ENCLOSURE_MANAGER_MV1100MT ManagerV1100ManagerType = "EnclosureManager"
	BMC_MV1100MT ManagerV1100ManagerType = "BMC"
	RACK_MANAGER_MV1100MT ManagerV1100ManagerType = "RackManager"
	AUXILIARY_CONTROLLER_MV1100MT ManagerV1100ManagerType = "AuxiliaryController"
	SERVICE_MV1100MT ManagerV1100ManagerType = "Service"
)