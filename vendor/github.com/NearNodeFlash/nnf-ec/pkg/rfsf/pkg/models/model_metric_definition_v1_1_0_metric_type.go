/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi
// MetricDefinitionV110MetricType : The type of metric.  Provides information to the client on how the metric can be handled.
type MetricDefinitionV110MetricType string

// List of MetricDefinition_v1_1_0_MetricType
const (
	NUMERIC_MDV110MT MetricDefinitionV110MetricType = "Numeric"
	DISCRETE_MDV110MT MetricDefinitionV110MetricType = "Discrete"
	GAUGE_MDV110MT MetricDefinitionV110MetricType = "Gauge"
	COUNTER_MDV110MT MetricDefinitionV110MetricType = "Counter"
	COUNTDOWN_MDV110MT MetricDefinitionV110MetricType = "Countdown"
)
