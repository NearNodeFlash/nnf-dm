/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MemoryV1100MemoryLocation - Memory connection information to sockets and memory controllers.
type MemoryV1100MemoryLocation struct {

	// The channel number to which the memory device is connected.
	Channel int64 `json:"Channel,omitempty"`

	// The memory controller number to which the memory device is connected.
	MemoryController int64 `json:"MemoryController,omitempty"`

	// The slot number to which the memory device is connected.
	Slot int64 `json:"Slot,omitempty"`

	// The socket number to which the memory device is connected.
	Socket int64 `json:"Socket,omitempty"`
}
