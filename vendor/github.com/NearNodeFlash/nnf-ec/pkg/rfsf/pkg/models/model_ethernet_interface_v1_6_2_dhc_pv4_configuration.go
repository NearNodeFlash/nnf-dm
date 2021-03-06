/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// EthernetInterfaceV162DhcPv4Configuration - DHCPv4 configuration for this interface.
type EthernetInterfaceV162DhcPv4Configuration struct {

	// An indication of whether DHCP v4 is enabled on this Ethernet interface.
	DHCPEnabled bool `json:"DHCPEnabled,omitempty"`

	FallbackAddress EthernetInterfaceV162DHCPFallback `json:"FallbackAddress,omitempty"`

	// An indication of whether this interface uses DHCP v4-supplied DNS servers.
	UseDNSServers bool `json:"UseDNSServers,omitempty"`

	// An indication of whether this interface uses a DHCP v4-supplied domain name.
	UseDomainName bool `json:"UseDomainName,omitempty"`

	// An indication of whether this interface uses a DHCP v4-supplied gateway.
	UseGateway bool `json:"UseGateway,omitempty"`

	// An indication of whether the interface uses DHCP v4-supplied NTP servers.
	UseNTPServers bool `json:"UseNTPServers,omitempty"`

	// An indication of whether the interface uses DHCP v4-supplied static routes.
	UseStaticRoutes bool `json:"UseStaticRoutes,omitempty"`
}
