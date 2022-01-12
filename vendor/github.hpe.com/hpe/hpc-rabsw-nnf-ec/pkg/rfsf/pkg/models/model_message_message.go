/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// MessageMessage - The message that the Redfish service returns.
type MessageMessage struct {

	// The human-readable message, if provided.
	Message string `json:"Message,omitempty"`

	// This array of message arguments are substituted for the arguments in the message when looked up in the message registry.
	MessageArgs []string `json:"MessageArgs,omitempty"`

	// The key for this message used to find the message in a message registry.
	MessageId string `json:"MessageId"`

	// The OEM extension.
	Oem map[string]interface{} `json:"Oem,omitempty"`

	// A set of properties described by the message.
	RelatedProperties []string `json:"RelatedProperties,omitempty"`

	// Used to provide suggestions on how to resolve the situation that caused the error.
	Resolution string `json:"Resolution,omitempty"`

	// The severity of the errors.
	Severity string `json:"Severity,omitempty"`

	MessageSeverity ResourceHealth `json:"MessageSeverity,omitempty"`
}
