/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

import (
	"time"
)

// UpdateServiceV182HttpPushUriApplyTime - The settings for when to apply HttpPushUri-provided software.
type UpdateServiceV182HttpPushUriApplyTime struct {

	ApplyTime UpdateServiceV182ApplyTime `json:"ApplyTime,omitempty"`

	// The expiry time, in seconds, of the maintenance window.
	MaintenanceWindowDurationInSeconds int64 `json:"MaintenanceWindowDurationInSeconds,omitempty"`

	// The start time of a maintenance window.
	MaintenanceWindowStartTime time.Time `json:"MaintenanceWindowStartTime,omitempty"`
}
