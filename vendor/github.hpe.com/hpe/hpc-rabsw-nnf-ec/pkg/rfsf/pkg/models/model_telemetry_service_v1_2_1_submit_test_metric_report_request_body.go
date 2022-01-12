/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// TelemetryServiceV121SubmitTestMetricReportRequestBody - This action generates a metric report.
type TelemetryServiceV121SubmitTestMetricReportRequestBody struct {

	// The content of the MetricReportValues in the generated metric report.
	GeneratedMetricReportValues []TelemetryServiceV121MetricValue `json:"GeneratedMetricReportValues"`

	// The name of the metric report in generated metric report.
	MetricReportName string `json:"MetricReportName"`

	// The contents of MetricReportValues array in the generated metric report.
	MetricReportValues string `json:"MetricReportValues,omitempty"`
}
