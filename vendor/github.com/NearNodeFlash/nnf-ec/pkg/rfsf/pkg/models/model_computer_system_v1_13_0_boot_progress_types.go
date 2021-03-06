/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type ComputerSystemV1130BootProgressTypes string

// List of ComputerSystem_v1_13_0_BootProgressTypes
const (
	NONE_CSV1130BPT ComputerSystemV1130BootProgressTypes = "None"
	PRIMARY_PROCESSOR_INITIALIZATION_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "PrimaryProcessorInitializationStarted"
	BUS_INITIALIZATION_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "BusInitializationStarted"
	MEMORY_INITIALIZATION_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "MemoryInitializationStarted"
	SECONDARY_PROCESSOR_INITIALIZATION_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "SecondaryProcessorInitializationStarted"
	PCI_RESOURCE_CONFIG_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "PCIResourceConfigStarted"
	SYSTEM_HARDWARE_INITIALIZATION_COMPLETE_CSV1130BPT ComputerSystemV1130BootProgressTypes = "SystemHardwareInitializationComplete"
	OS_BOOT_STARTED_CSV1130BPT ComputerSystemV1130BootProgressTypes = "OSBootStarted"
	OS_RUNNING_CSV1130BPT ComputerSystemV1130BootProgressTypes = "OSRunning"
	OEM_CSV1130BPT ComputerSystemV1130BootProgressTypes = "OEM"
)
