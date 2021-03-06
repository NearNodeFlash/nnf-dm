/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// PortV130LinkConfiguration - Properties of the link for which this port is configured.
type PortV130LinkConfiguration struct {

	// An indication of whether the port is capable of autonegotiating speed.
	AutoSpeedNegotiationCapable bool `json:"AutoSpeedNegotiationCapable,omitempty"`

	// Controls whether this port is configured to enable autonegotiating speed.
	AutoSpeedNegotiationEnabled bool `json:"AutoSpeedNegotiationEnabled,omitempty"`

	// The set of link speed capabilities of this port.
	CapableLinkSpeedGbps []float32 `json:"CapableLinkSpeedGbps,omitempty"`

	// The set of link speed and width pairs this port is configured to use for autonegotiation.
	ConfiguredNetworkLinks []PortV130ConfiguredNetworkLink `json:"ConfiguredNetworkLinks,omitempty"`
}
