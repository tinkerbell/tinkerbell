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

This mode is used to provide next boot information to clients without requiring a pre-existing Hardware object. In this mode, Tinkerbell will respond to PXE enabled DHCP requests from clients and provide them with next boot info when network booting. All network booting clients will be provided the next boot info for the iPXE binary. When a client needs an iPXE script, if no corresponding Hardware object is found for the requesting client's MAC address, Tinkerbell will provide the client with a statically defined iPXE script. If a Hardware record is found, then the normal `auto.ipxe` script will be served for IPv4. In this mode Tinkerbell does NOT provide IP addresses to clients, it only provides next boot information. A DHCP server on the network must be configured to provide IP addresses to clients. Tinkerbell requires Layer 2 access to machines or a DHCP relay agent that will forward DHCP requests to Tinkerbell.

#### Auto Proxy DHCP Configuration

To enable this mode set the CLI flag `--dhcp-mode=auto-proxy` or the environment variable `TINKERBELL_DHCP_MODE=auto-proxy`.

### DHCP Disabled

This mode is used to disable all DHCP functionality in Tinkerbell. In this mode, the user is required to handle all DHCP functionality.

#### DHCP Disabled Configuration

To enable this mode set the CLI flag `--dhcp-enabled=false` or the environment variable `TINKERBELL_DHCP_ENABLED=false`.

## Interoperability with other DHCP servers

When a DHCP server exists on the network, Tinkerbell should be set to run `proxy` or `auto-proxy` mode. This will allow Tinkerbell to provide the next boot information to clients that request it and the existing DHCP server will provide IP address information. Layer 2 access to machines or a DHCP relay agent that will forward the DHCP requests to Tinkerbell is required.

## Address Reservation Limitations

Each Hardware interface can define only one reserved IP address. That address can be either IPv4 or IPv6, but not both. DHCPv4 reservation mode uses only IPv4 Hardware addresses and ignores Hardware records whose reserved address is IPv6. DHCPv6 reservation mode uses only IPv6 Hardware addresses and ignores Hardware records whose reserved address is IPv4.

## DHCPv6 Client Identification

Tinkerbell DHCPv6 mode requires a reliable way to identify the client MAC address.

For direct client traffic, Tinkerbell can identify the client when at least one of the following is true:

* the client source IPv6 address is derived from the MAC address using EUI-64;
* the client DUID exposes a link-layer address, for example DUID-LL or DUID-LLT;

For relayed traffic, the relay should include DHCPv6 Option 79, Client Link-Layer Address, in the Relay-Forward message. If Option 79 is not available, Tinkerbell may only infer the MAC address from the peer address when that address is link-local and EUI-64-derived.

Clients are ignored when Tinkerbell cannot determine a stable client MAC address. This includes clients that use opaque or privacy-style IPv6 addresses, use a DUID that does not expose a link-layer address, and are not relayed with Option 79 or another trusted identity mapping.

Without a client MAC address, Tinkerbell cannot reliably match the request to Hardware data, inject the MAC into boot URLs, or route the client to the correct fallback script.

## DHCPv6 Modes

### Stateless DHCPv6

This mode is used to provide DHCPv6 stateless configuration to clients that already have a matching Hardware object. Addressing remains outside Tinkerbell and is expected to come from Router Advertisements and SLAAC. In this mode, Tinkerbell replies when a corresponding Hardware interface exists for the client's MAC address and can be converted into DHCP and netboot data. The client does not need to request the Boot File URL option for Tinkerbell to reply with other stateless information, such as DNS, domain search, or NTP servers from the Hardware DHCP configuration. `allowPXE` controls boot permission only; it does not decide whether DHCPv6 stateless configuration is served.

To enable this mode set the CLI flag `--dhcpv6-mode=stateless` or the environment variable `TINKERBELL_DHCPV6_MODE=stateless`.

### Auto-Stateless DHCPv6

This mode is the DHCPv6 analogue of DHCPv4 `auto-proxy` for clients that do not yet have a Hardware object. Addressing still comes from Router Advertisements and SLAAC, but Tinkerbell will reply to valid Information-request messages even when no Hardware object exists yet. This is intentional even for unknown clients that do not request the Boot File URL option; Tinkerbell still provides a DHCPv6 stateless reply with server identity and refresh timing, while omitting boot data unless the client requested it. Unknown clients receive the default iPXE binary for their architecture when they request the Boot File URL option, and unknown second-stage iPXE requests are directed to the static fallback script. When a Hardware object exists, its custom boot data and stateless DHCP information are still used. If the Hardware object cannot be converted into usable DHCP and netboot data, Tinkerbell ignores the client instead of falling back to unknown-client defaults. If it explicitly disables DHCP, Tinkerbell suppresses responses for that client. If it disallows netboot, Tinkerbell still serves DHCPv6 stateless configuration but returns a valid not-allowed Boot File URL when the client requests Option 59.

To enable this mode set the CLI flag `--dhcpv6-mode=auto-stateless` or the environment variable `TINKERBELL_DHCPV6_MODE=auto-stateless`.

### Reservation DHCPv6

This mode is the DHCPv6 analogue of DHCPv4 `reservation`. Tinkerbell replies only when a matching Hardware object exists for the client's MAC address and that Hardware object has a reserved IPv6 address.

Tinkerbell serves the selected address with IA_NA for Solicit, Request, Renew, and Rebind flows, and it supports both direct DHCPv6 messages and relay-forward wrapped messages. Solicit, Request, Renew, and Rebind messages without an IA_NA option are ignored. A Request IA_NA that contains no IAAddr is treated as a request for the selected reservation. Request messages that include a stale or different IA_NA address receive a Reply with an IA_NA status of NoAddrsAvail; Renew and Rebind messages that include a stale or different IA_NA address receive NoBinding. An IA_NA with client-provided addresses matches the host reservation only when it contains exactly one IAAddr and that IAAddr is the selected reservation. Confirm messages are ignored because Tinkerbell does not maintain enough link state to validate arbitrary client-held addresses. Release and Decline messages receive a Reply with an IA_NA status of Success when the requested address matches the host reservation, or NoBinding when it does not.

This exact single-address match for client-provided addresses is a Tinkerbell reservation-mode policy, not a general DHCPv6 protocol limit. DHCPv6 allows an IA_NA to carry zero, one, or more IAAddr options in a Request, and servers may assign addresses that differ from the client's requested addresses. Tinkerbell Hardware reservations define one authoritative IPv6 address per interface, so empty Request IA_NA options are assigned the selected reservation, while mixed requested address sets are treated as non-matches to avoid accepting client state that includes addresses outside the host reservation.

Tinkerbell sets each IAAddr preferred lifetime to half of the valid lifetime, IA_NA T1 to half of the valid lifetime, and IA_NA T2 to 80% of the valid lifetime.

Reservation DHCPv6 can be used on networks where Router Advertisements set `M=1`, `O=1`, and `A=1`. Tinkerbell does not send or configure Router Advertisements; RA and SLAAC behavior remain managed by the network.

To enable this mode set the CLI flag `--dhcpv6-mode=reservation` or the environment variable `TINKERBELL_DHCPV6_MODE=reservation`.

### Derived DHCPv6

This mode replies only when a matching Hardware object exists for the client's MAC address. If the Hardware object has a reserved IPv6 address, Tinkerbell serves that address. If it does not, Tinkerbell derives a stable temporary address from the client's MAC address and either a direct-request pool or the relay-forward link-address.

Derived mode is intended for boot-time compatibility with systems that do not properly work with SLAAC when Router Advertisements set `M=0`, but still send DHCPv6 Solicit requests. Derived addresses are temporary boot-only addresses. Do not use derived mode to configure static IP addresses for machines, and do not treat these addresses as persisted leases.

Derived mode has no lease database and does not track Duplicate Address Detection (DAD) failures. If a client receives a derived address, detects a duplicate address, and sends a DHCPv6 Decline, Tinkerbell replies according to the DHCPv6 Decline flow but does not mark the address as unusable or choose a replacement. Because derived addresses are deterministic from the client MAC address and selected prefix, the same client will receive the same derived address again until the duplicate is removed or the derived prefix configuration changes.

Direct client requests use `--dhcpv6-derived-direct-address-pool` / `TINKERBELL_DHCPV6_DERIVED_DIRECT_ADDRESS_POOL`. The prefix must be a usable IPv6 unicast prefix between `/1` and `/64`; `/65` through `/128` are rejected because they do not leave enough host bits for stateless MAC-based derivation, and unusable ranges such as unspecified, IPv4-mapped, link-local, or multicast prefixes are rejected. If no direct pool is configured, derived mode ignores direct requests that do not already have a Hardware IPv6 reservation.

Relayed requests use the relay-forward `link-address` as the source prefix and `--dhcpv6-derived-relay-address-prefix` / `TINKERBELL_DHCPV6_DERIVED_RELAY_ADDRESS_PREFIX` as the prefix length. The relay prefix length defaults to `/64` and must be between `/1` and `/64`; `/0` and `/65` through `/128` are rejected. The relay must set a usable IPv6 link-address for the client link.

To enable this mode set the CLI flag `--dhcpv6-mode=derived` or the environment variable `TINKERBELL_DHCPV6_MODE=derived`.

### DHCPv6 Boot URL Selection

For DHCPv6 stateless, auto-stateless, reservation, and derived modes, Tinkerbell includes DHCPv6 Option 59, Boot File URL, only when the client requests that option. Tinkerbell can still answer requests that only ask for other stateless information, such as DNS. When Option 59 is requested, Tinkerbell uses RFC 5970 boot options to decide which boot URL to return. If a matching Hardware interface has netboot data with `allowPXE: false`, or netboot data that omits `allowPXE`, Tinkerbell returns a valid not-allowed URL rooted at the configured DHCPv6 iPXE script URL. If Tinkerbell cannot determine any boot URL for a request that asked for Option 59, it ignores the request instead of sending DHCP data with unusable netboot data. The primary signal for HTTP boot is DHCPv6 Option 61, Client System Architecture Type. When the client reports an HTTP boot architecture, Tinkerbell returns an HTTP iPXE binary URL in Option 59.

Vendor Class data is also used as an HTTP boot hint. Tinkerbell treats a Vendor Class value containing `HTTPClient` as an HTTP boot client signal, even when the client also reports a non-HTTP architecture. `HTTPClient` does not need to be the first token in the Vendor Class data.

IPv6 boot flows use the IPv6-specific Tinkerbell options for advertised boot and iPXE script values. Configure the `--public-ipv6`, `--dhcpv6-*`, `--ipxe-http-script-osie-url-v6`, and `--ipxe-script-tink-server-addr-port-v6` settings as needed for IPv6 clients. Do not rely on a dual-stack DNS name in the IPv4/common options to make the IPv6 boot path work; the IPv6 path does not automatically fall back to those values.

For stable DHCPv6 client behavior, configure a stable server DUID with `--dhcpv6-server-duid` / `TINKERBELL_DHCPV6_SERVER_DUID`. The value is the complete DHCPv6 DUID encoded as raw hex bytes, with optional `:` or `-` separators. For example, a DUID-UUID value starts with `00:04` followed by the 16 UUID bytes. If this option is empty, Tinkerbell keeps its automatic fallback behavior and derives a DUID from the configured IPv6 Tink Server address when possible, otherwise it uses a built-in fallback DUID. In Kubernetes, prefer storing the configured DUID in a Secret or other release-stable configuration rather than deriving it from Pod, Node, or Service addresses.

### Hardware URL Overrides

Hardware-specific netboot and OSIE URL overrides are used as supplied. Tinkerbell does not rewrite or validate these URLs against the client's IP stack. Callers are responsible for configuring override URLs that are reachable by the intended boot path. For example, IPv6-only boot flows need Hardware override URLs that are IPv6-reachable, while IPv4-only boot flows need IPv4-reachable URLs.
