/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// EventDestinationV190Actions - The available actions for this Resource.
type EventDestinationV190Actions struct {

	EventDestinationResumeSubscription EventDestinationV190ResumeSubscription `json:"#EventDestination.ResumeSubscription,omitempty"`

	// The available OEM-specific actions for this Resource.
	Oem map[string]interface{} `json:"Oem,omitempty"`
}
