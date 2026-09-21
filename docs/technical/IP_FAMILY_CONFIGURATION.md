# IP Family Configuration

Tinkerbell runs in one of three topologies: single stack IPv4, single stack
IPv6, or dual stack. This document describes how to configure each, and how to
confirm which one you got.

## Flag naming

Every setting that can differ per address family carries a family suffix.

| Suffix | Meaning |
| --- | --- |
| `-v4` | applies to IPv4 only |
| `-v6` | applies to IPv6 only |
| *(none)* | applies to both families |

A flag with no suffix is deliberately family independent, not an IPv4 default.
`--iso-upstream-url`, `--tftp-asset-dir`, and `--ipxe-http-script-retries` are
the same for every machine. `--trusted-proxies` and
`--ipxe-http-script-trusted-proxies` accept CIDRs of either family in one list.

A handful of settings exist for one family only, because the protocols differ:

| Flag | Why there is no counterpart |
| --- | --- |
| `--dhcp-ip-for-packet-v4` | DHCPv4 option 54 names the server by address; DHCPv6 uses a DUID. |
| `--dhcp-server-duid-v6` | DHCPv4 has no DUID. |
| `--dhcp-derived-direct-address-pool-v6` | Derived addressing is DHCPv6 only. |
| `--dhcp-derived-relay-address-prefix-v6` | Derived addressing is DHCPv6 only. |
| `--iso-static-ipam-enabled-v4` | Static IPAM in patched ISOs is IPv4 only. IPv6 machines use SLAAC or DHCPv6. |

Every other `-v4` flag has a `-v6` counterpart and vice versa. A missing
counterpart is a bug, and a test enforces it.

## Addresses to bind versus addresses to advertise

Two different questions, two different flags:

- `--bind-address-v4` / `--bind-address-v6` are the local addresses Tinkerbell
  listens on. They must exist on a local interface.
- `--public-ip-v4` / `--public-ip-v6` are the addresses written into DHCP
  options, iPXE scripts, and patched ISOs, so machines and agents know where to
  call back. They may be a load balancer, NAT, or ingress address that exists
  nowhere on the host.

A family is served when it has a bind address, and advertised when it has a
public address. These are independent: you can bind `::` while advertising a
different routable IPv6 address.

## Single stack IPv4

Set only the `-v4` settings. This is the default when the host has no global
IPv6 address.

```console
tinkerbell \
  --public-ip-v4 192.0.2.10 \
  --bind-address-v4 0.0.0.0 \
  --dhcp-enabled-v4 \
  --dhcp-enabled-v6=false
```

## Single stack IPv6

Set only the `-v6` settings, and leave `--bind-address-v4` unset so nothing
binds for IPv4.

```console
tinkerbell \
  --public-ip-v6 2001:db8::10 \
  --bind-address-v6 :: \
  --dhcp-enabled-v4=false \
  --dhcp-enabled-v6 \
  --dhcp-bind-interface-v6 eth0 \
  --ipxe-http-script-osie-url-v6 http://[2001:db8::20]/hook
```

Notes for IPv6-only deployments:

- `--dhcp-bind-interface-v6` is required in most deployments. DHCPv6 relies on
  the `ff02::1:2` multicast group, which has to be joined on a specific
  interface. It accepts a comma-separated list.
- `--iso-static-ipam-enabled-v4` has no IPv6 equivalent. IPv6 machines get
  addresses from SLAAC or DHCPv6, so requests to the IPv6 ISO route are
  rejected while static IPAM is enabled.
- IPv6 literals in URLs and `addr:port` values must be bracketed, for example
  `[2001:db8::10]:42113`.

## Dual stack

Set both families. An IPv4 and an IPv6 listener can share a port, because each
socket is bound to a single family.

```console
tinkerbell \
  --public-ip-v4 192.0.2.10 \
  --public-ip-v6 2001:db8::10 \
  --bind-address-v4 0.0.0.0 \
  --bind-address-v6 :: \
  --dhcp-enabled-v4 \
  --dhcp-enabled-v6 \
  --dhcp-bind-interface-v6 eth0 \
  --ipxe-http-script-osie-url-v4 http://192.0.2.20/hook \
  --ipxe-http-script-osie-url-v6 http://[2001:db8::20]/hook
```

Dual stack means each family is configured end to end. Setting
`--public-ip-v6` alone does not make a deployment dual stack: without
`--bind-address-v6` nothing listens on IPv6, and IPv6 machines have nothing to
reach.

Kernel command line arguments are per family. `--ipxe-http-script-extra-kernel-args-v4`
and `--ipxe-http-script-extra-kernel-args-v6` are separate, so `tink_worker_image`
or `insecure_registries` can differ between IPv4 and IPv6 machines. Set both if
they should match.

## Confirming the result

At startup Tinkerbell logs the topology of each service:

```json
{"level":"0","msg":"address families","logger":"cli",
 "advertised":"dual-stack","http":"dual-stack","dhcp":"dual-stack",
 "syslog":"dual-stack","tftp":"dual-stack","tinkServer":"dual-stack",
 "secondstar":"dual-stack"}
```

Each value is `ipv4-only`, `ipv6-only`, `dual-stack`, or `none`. A service
reporting `ipv4-only` when you expected `dual-stack` means that family has no
bind address; a service reporting `none` is not listening at all.

An address whose family contradicts its flag is rejected at startup:

```console
$ tinkerbell --dhcp-syslog-ip-v4 2001:db8::10
--dhcp-syslog-ip-v4: "2001:db8::10" is not an IPv4 address
```

IPv4-mapped IPv6 addresses such as `::ffff:192.0.2.10` reach IPv4 peers and are
rejected by `-v6` flags.

## Environment variables and Helm

Every flag has an environment variable: `TINKERBELL_` plus the flag name
uppercased with `-` replaced by `_`. `--dhcp-bind-addr-v6` reads
`TINKERBELL_DHCP_BIND_ADDR_V6`.

The Helm chart exposes these under `deployment.envs`, with `V6`-suffixed keys
alongside their IPv4 counterparts:

```yaml
deployment:
  envs:
    globals:
      bindAddr: "0.0.0.0"
      bindAddrV6: "::"
      publicIpv4: "192.0.2.10"
      publicIpv6: "2001:db8::10"
    smee:
      dhcpEnabled: true
      dhcpv6Enabled: true
      dhcpv6BindInterface: "eth0"
```

## Upgrading

Flag and environment variable names that did not carry a family suffix still
work and set the same values. They log a deprecation warning at startup naming
the replacement, and will be removed in a future release.

One rename is not one-to-one: `--ipxe-http-script-extra-kernel-args` reached
machines of both families, so it sets both
`--ipxe-http-script-extra-kernel-args-v4` and
`--ipxe-http-script-extra-kernel-args-v6`.
