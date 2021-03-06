/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type ChassisV1140IntrusionSensor string

// List of Chassis_v1_14_0_IntrusionSensor
const (
	NORMAL_CV1140IS ChassisV1140IntrusionSensor = "Normal"
	HARDWARE_INTRUSION_CV1140IS ChassisV1140IntrusionSensor = "HardwareIntrusion"
	TAMPERING_DETECTED_CV1140IS ChassisV1140IntrusionSensor = "TamperingDetected"
)
