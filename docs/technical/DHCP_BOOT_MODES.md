# DHCP Boot Modes

This document explains the different DHCP boot modes available in Tinkerbell and how to configure them.

## Overview

As requirements across users can vary, flexibility in DHCP boot modes is essential for accommodating different network environments and client needs.
Tinkerbell provides several DHCP boot modes to cater to these requirements. The modes include DHCP Reservation, Proxy DHCP, Auto Proxy DHCP, and DHCP disabled. Each mode has its own use case and configuration options.

## DHCP Modes

### DHCP Reservation

This mode is used to provide IP addresses and next boot information to clients based on their MAC addresses. In this mode, the IP address is reservation-based, meaning there must be a corresponding Hardware object for the requesting client's MAC address.

#### DHCP Reservation Configuration

This is the default mode. To explicitly enable this mode use the CLI flag `--dhcp-mode=reservation` or the environment variable `TINKERBELL_DHCP_MODE=reservation`.

### Proxy DHCP

This mode is used to provide next boot information to clients. In this mode, a Hardware object must exist for the requesting client's MAC address. In this mode Tinkerbell does NOT provide IP addresses to clients, it only provides next boot information. A DHCP server on the network must be configured to provide IP addresses to clients. Tinkerbell requires Layer 2 access to machines or a DHCP relay agent that will forward DHCP requests to Tinkerbell.

#### Proxy DHCP Configuration

To enable this mode set the CLI flag `--dhcp-mode=proxy` or the environment variable `TINKERBELL_DHCP_MODE=proxy`.

### Auto Proxy DHCP

This mode is used to provide next boot information to clients without requiring a pre-existing Hardware object. In this mode, Tinkerbell will respond to PXE enabled DHCP requests from clients and provide them with next boot info when network booting. All network booting clients will be provided the next boot info for the iPXE binary. When a client needs an iPXE script, if no corresponding Hardware object is found for the requesting client's MAC address, Tinkerbell will provide the client with a statically defined iPXE script. If a Hardware record is found, then the normal `auto.ipxe` script will be served. In this mode Tinkerbell does NOT provide IP addresses to clients, it only provides next boot information. A DHCP server on the network must be configured to provide IP addresses to clients. Tinkerbell requires Layer 2 access to machines or a DHCP relay agent that will forward DHCP requests to Tinkerbell.

#### Auto Proxy DHCP Configuration

To enable this mode set the CLI flag `--dhcp-mode=auto-proxy` or the environment variable `TINKERBELL_DHCP_MODE=auto-proxy`.

### DHCP Disabled

This mode is used to disable all DHCP functionality in Tinkerbell. In this mode, the user is required to handle all DHCP functionality.

#### DHCP Disabled Configuration

To enable this mode set the CLI flag `--dhcp-enabled=false` or the environment variable `TINKERBELL_DHCP_ENABLED=false`.

## Interoperability with other DHCP servers

When a DHCP server exists on the network, Tinkerbell should be set to run `proxy` or `auto-proxy` mode. This will allow Tinkerbell to provide the next boot information to clients that request it and the existing DHCP server will provide IP address information. Layer 2 access to machines or a DHCP relay agent that will forward the DHCP requests to Tinkerbell is required.
