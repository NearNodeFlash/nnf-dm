/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// TaskV150Payload - The HTTP and JSON payload details for this Task.
type TaskV150Payload struct {

	// An array of HTTP headers that this task includes.
	HttpHeaders []string `json:"HttpHeaders,omitempty"`

	// The HTTP operation to perform to execute this task.
	HttpOperation string `json:"HttpOperation,omitempty"`

	// The JSON payload to use in the execution of this task.
	JsonBody string `json:"JsonBody,omitempty"`

	// The URI of the target for this task.
	TargetUri string `json:"TargetUri,omitempty"`
}
