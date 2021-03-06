/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// ComputerSystemV1130HostGraphicalConsole - The information about a graphical console service for this system.
type ComputerSystemV1130HostGraphicalConsole struct {

	// This property enumerates the graphical console connection types that the implementation allows.
	ConnectTypesSupported []ComputerSystemV1130GraphicalConnectTypesSupported `json:"ConnectTypesSupported,omitempty"`

	// The maximum number of service sessions, regardless of protocol, that this system can support.
	MaxConcurrentSessions int64 `json:"MaxConcurrentSessions,omitempty"`

	// The protocol port.
	Port int64 `json:"Port,omitempty"`

	// An indication of whether the service is enabled for this system.
	ServiceEnabled bool `json:"ServiceEnabled,omitempty"`
}
