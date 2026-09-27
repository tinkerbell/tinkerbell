# IP Family Configuration

Tinkerbell runs in one of three topologies: single stack IPv4, single stack
IPv6, or dual stack. This document describes how to configure each, and how to
confirm which one you got.

## Selecting the families

`--listener-families` selects the address families every listener serves. It
accepts `ipv4` (the default), `ipv6`, or `dual`.

This is a ceiling, not a switch for individual services: a service can narrow
the set with its own enable flag, but never widen it. `--dhcp-enabled-v6=false`
under `--listener-families dual` leaves DHCPv6 off while HTTP, TFTP, syslog,
gRPC, and SSH still serve both families.

Settings scoped to a family that is not served are **ignored, not rejected**.
One set of flags or one Helm values file can therefore describe both families,
and moving between topologies is a one-line change. The ignored settings are
named at startup:

```json
{"level":"0","msg":"ignoring flags for address families not being served",
 "logger":"cli","listenerFamilies":"ipv4",
 "ignoredFlags":["bind-address-v6","public-ip-v6"]}
```

Public address auto-detection follows the same rule: an unserved family is
never detected and never advertised.

## Flag naming

Every setting that can differ per address family carries a family suffix.

| Suffix | Meaning |
| --- | --- |
| `-v4` | applies to IPv4 only |
| `-v6` | applies to IPv6 only |
| *(none)* | applies to both families |

A flag with no suffix is deliberately family independent, not an IPv4 default.
`--iso-upstream-url`, `--tftp-asset-dir`, and `--ipxe-http-script-retries` are
the same for every machine. `--trusted-proxies` accepts CIDRs of either family
in one list.

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

Neither has to be set. For a served family, `--public-ip-*` defaults to an
address detected on the host, `--bind-address-v4` to the detected IPv4 address
or `0.0.0.0`, and `--bind-address-v6` to `::`. Binding `::` while advertising a
different routable IPv6 address is the expected shape behind a load balancer.

## Single stack IPv4

The default. Nothing has to be set beyond the public address, and even that is
detected when omitted.

```console
tinkerbell \
  --listener-families ipv4 \
  --public-ip-v4 192.0.2.10
```

## Single stack IPv6

```console
tinkerbell \
  --listener-families ipv6 \
  --public-ip-v6 2001:db8::10 \
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

An IPv4 and an IPv6 listener share a port number, because each socket is bound
to a single family and the IPv6 socket is always `IPV6_V6ONLY`.

```console
tinkerbell \
  --listener-families dual \
  --public-ip-v4 192.0.2.10 \
  --public-ip-v6 2001:db8::10 \
  --dhcp-bind-interface-v6 eth0 \
  --ipxe-http-script-osie-url-v4 http://192.0.2.20/hook \
  --ipxe-http-script-osie-url-v6 http://[2001:db8::20]/hook
```

Setting `--public-ip-v6` alone does not make a deployment dual stack. Under the
default `--listener-families ipv4` nothing listens on IPv6 and the value is
discarded.

Kernel command line arguments are per family. `--ipxe-http-script-extra-kernel-args-v4`
and `--ipxe-http-script-extra-kernel-args-v6` are separate, so `tink_worker_image`
or `insecure_registries` can differ between IPv4 and IPv6 machines. Set both if
they should match.

## Confirming the result

The `starting tinkerbell` log line reports the families each service ended up
serving, so the resolved plan does not have to be inferred from the flags:

```json
{"level":"0","msg":"starting tinkerbell","logger":"cli",
 "listenerFamilies":"dual",
 "servedFamilies":{"dhcp":"dual","http":"dual","secondstar":"dual",
                   "syslog":"dual","tftp":"dual","tink-server":"dual"}}
```

Each value is `ipv4`, `ipv6`, `dual`, or `none`. Only enabled services appear.
A service narrower than `listenerFamilies` was restricted by its own enable
flag; `none` means it is not listening at all.

An address whose family contradicts its flag is rejected during flag parsing:

```console
$ tinkerbell --dhcp-syslog-ip-v4 2001:db8::10
tinkerbell: parse args: --dhcp-syslog-ip-v4: set "2001:db8::10": "2001:db8::10" is not an IPv4 address
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
      listenerFamilies: dual
      publicIpv4: "192.0.2.10"
      publicIpv6: "2001:db8::10"
    smee:
      dhcpv6BindInterface: "eth0"
```

`service.ipFamilies` is separate: it controls the families Kubernetes assigns
to the Service, not the families the pod listens on. Set both. See the chart
[README](../../helm/tinkerbell/README.md).
