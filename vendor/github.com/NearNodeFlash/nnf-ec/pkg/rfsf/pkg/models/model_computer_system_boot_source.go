/*
 * Swordfish API
 *
 * This contains the definition of the Swordfish extensions to a Redfish service.
 *
 * API version: v1.2.c
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type ComputerSystemBootSource string

// List of ComputerSystem_BootSource
const (
	NONE_CSBS ComputerSystemBootSource = "None"
	PXE_CSBS ComputerSystemBootSource = "Pxe"
	FLOPPY_CSBS ComputerSystemBootSource = "Floppy"
	CD_CSBS ComputerSystemBootSource = "Cd"
	USB_CSBS ComputerSystemBootSource = "Usb"
	HDD_CSBS ComputerSystemBootSource = "Hdd"
	BIOS_SETUP_CSBS ComputerSystemBootSource = "BiosSetup"
	UTILITIES_CSBS ComputerSystemBootSource = "Utilities"
	DIAGS_CSBS ComputerSystemBootSource = "Diags"
	UEFI_SHELL_CSBS ComputerSystemBootSource = "UefiShell"
	UEFI_TARGET_CSBS ComputerSystemBootSource = "UefiTarget"
	SD_CARD_CSBS ComputerSystemBootSource = "SDCard"
	UEFI_HTTP_CSBS ComputerSystemBootSource = "UefiHttp"
	REMOTE_DRIVE_CSBS ComputerSystemBootSource = "RemoteDrive"
	UEFI_BOOT_NEXT_CSBS ComputerSystemBootSource = "UefiBootNext"
)
