/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type EventServiceV170SMTPConnectionProtocol string

// List of EventService_v1_7_0_SMTPConnectionProtocol
const (
	NONE_ESV170SMTPCP EventServiceV170SMTPConnectionProtocol = "None"
	AUTO_DETECT_ESV170SMTPCP EventServiceV170SMTPConnectionProtocol = "AutoDetect"
	START_TLS_ESV170SMTPCP EventServiceV170SMTPConnectionProtocol = "StartTLS"
	TLS_SSL_ESV170SMTPCP EventServiceV170SMTPConnectionProtocol = "TLS_SSL"
)