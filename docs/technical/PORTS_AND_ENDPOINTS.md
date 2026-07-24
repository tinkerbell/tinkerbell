# Tinkerbell Ports, Protocols, and HTTP Endpoints

This document describes all network ports, protocols, and HTTP endpoints used by
the Tinkerbell stack. All services run inside a single `tinkerbell` binary.
Individual services can be enabled or disabled via CLI flags or environment
variables.

---

## Listening Ports

| Port | Protocol | Service | Description | Flag to Disable |
|------|----------|---------|-------------|-----------------|
| **7080** | TCP (HTTP) | Consolidated HTTP server | All HTTP endpoints listed below | Always on |
| **7443** | TCP (HTTPS) | Consolidated HTTPS server | Same routes as HTTP; enabled when TLS cert/key are provided | `--tls-cert-file` / `--tls-key-file` |
| **42113** | TCP (gRPC) | Tink Server | Workflow service for tink-agent | `--enable-tink-server=false` |
| **67** | UDP | Smee DHCP | PXE boot: offers next-server, iPXE script URL, and IP configuration | `--enable-smee=false` |
| **547** | UDP | Smee DHCPv6 | IPv6 netboot: DHCPv6 stateless data, boot file URL, reservation, and derived address support | `--dhcpv6-enabled=false` |
| **69** | UDP | Smee TFTP | Serves iPXE firmware binaries to PXE-booting machines | `--enable-smee=false` |
| **514** | UDP | Smee Syslog | Collects boot-time syslog messages from provisioning machines | `--enable-smee=false` |
| **2222** | TCP (SSH) | SecondStar | SSH-to-serial bridge for out-of-band hardware management via BMC | `--enable-secondstar=false` |

> **Note:** The HTTP and HTTPS ports are configurable via `--http-port` and
> `--https-port`. The gRPC port is configurable via `--tink-server-bind-port`.

Shared listeners support IPv4 and IPv6 bind addresses through
`--bind-address`. Use `--public-ipv4` and `--public-ipv6` to advertise the
addresses provisioned machines and agents should call back to.

### Bind Address Selection

When `--bind-address` is set, Tinkerbell uses that address for shared services
such as HTTP, Tink Server gRPC, TFTP, syslog, and SecondStar.

When `--bind-address` is not set, Tinkerbell chooses a default from the detected
or configured public addresses:

| Public IPv4 | Public IPv6 | `--dual-stack` | Default bind address |
|-------------|-------------|----------------|----------------------|
| yes | no | either | public IPv4 |
| no | yes | either | `::` |
| yes | yes | false | public IPv4 |
| yes | yes | true | `::` |
| no | no | either | `0.0.0.0` |

The `--dual-stack` flag only changes the default when both public IPv4 and
public IPv6 are available. It opts shared services into binding the IPv6
wildcard address. Whether an IPv6 wildcard socket also accepts IPv4 traffic is
platform dependent; on Linux it depends on `IPV6_V6ONLY` and
`net.ipv6.bindv6only`, and container or Kubernetes networking may impose
additional behavior.

The DHCPv6 listener has its own `--dhcpv6-bind-addr` setting and defaults to
`::` independently of `--dual-stack`. `--dhcpv6-bind-interface` can be set to
one interface or a comma-separated list of interfaces.

---

## HTTP / HTTPS Endpoints

All HTTP endpoints are served on the consolidated HTTP server (default `:7080`).
When TLS is configured, a subset of routes is also served on HTTPS (`:7443`);
these are marked with ✅ in the **HTTPS** column below.

Some HTTPS-enabled routes automatically redirect HTTP requests to HTTPS; these
are marked with ✅ in the **Redirect** column. These redirects can be disabled
with `--disable-http-to-https-redirect`.

### Health & Probes

| Route | Method | HTTPS | Redirect | Service | Description |
|-------|--------|-------|----------|---------|-------------|
| `/healthcheck` | GET | | | HTTP server | JSON response with `git_rev`, `uptime_seconds`, `goroutines` |
| `/healthz` | GET | | | HTTP server | Kubernetes-style liveness probe (returns `ok`) |
| `/readyz` | GET | | | HTTP server | Kubernetes-style readiness probe (returns `ok`) |

### Prometheus Metrics

Each service registers metrics on its own Prometheus registry, enabling
per-service scraping. A combined endpoint gathers from all registries.

| Route | Service | Metrics Served |
|-------|---------|----------------|
| `/metrics` | All | Combined: all service metrics + Go runtime + process collectors |
| `/smee/metrics` | Smee | `dhcp_total`, `discover_duration_seconds`, `discover_total`, `discover_in_progress`, `jobs_duration_seconds`, `jobs_total`, `jobs_in_progress` |
| `/tink-server/metrics` | Tink Server | `grpc_server_started_total`, `grpc_server_handled_total`, `grpc_server_handling_seconds`, `grpc_server_msg_received_total`, `grpc_server_msg_sent_total` |
| `/controllers/metrics` | Tink Controller + Rufio | controller-runtime metrics: work queue depth/latency, reconciliation duration/count, leader election, client-go cache metrics |
| `/http/metrics` | HTTP middleware | `http_server_requests_total`, `http_server_request_duration_seconds` |

### Boot & Provisioning (Smee)

| Route | Method | HTTPS | Redirect | Description |
|-------|--------|-------|----------|-------------|
| `/ipxe/binary/` | GET | | | Serves architecture-specific iPXE firmware binaries (e.g. `snp.efi`, `undionly.kpxe`). DHCP option 67 points machines here. |
| `/ipxe/script/` | GET | | | Serves auto-generated iPXE boot scripts. Supports MAC-address injection in the URL path (e.g. `/ipxe/script/aa:bb:cc:dd:ee:ff/auto.ipxe` for IPv4 and `/ipxe/script/aa:bb:cc:dd:ee:ff/auto6.ipxe` for IPv6). |
| `/iso/` | GET | ✅ | | Serves dynamically-patched ISO images with per-machine kernel parameters baked in. Enabled via `--smee-iso-enabled`. |

### EC2-Compatible Metadata (Tootles)

All metadata routes identify the requesting machine by its source IP address
(respecting `X-Forwarded-For` when trusted proxies are configured).

| Route | Method | HTTPS | Redirect | Description |
|-------|--------|-------|----------|-------------|
| `/2009-04-04/` | GET | ✅ | ✅ | EC2-compatible metadata root. Lists `user-data` and `meta-data`. |
| `/2009-04-04/user-data` | GET | ✅ | ✅ | Cloud-init user data for the machine. |
| `/2009-04-04/meta-data/instance-id` | GET | ✅ | ✅ | Hardware instance ID. |
| `/2009-04-04/meta-data/hostname` | GET | ✅ | ✅ | FQDN hostname. |
| `/2009-04-04/meta-data/local-hostname` | GET | ✅ | ✅ | Local hostname. |
| `/2009-04-04/meta-data/iqn` | GET | ✅ | ✅ | iSCSI Qualified Name. |
| `/2009-04-04/meta-data/plan` | GET | ✅ | ✅ | Facility plan slug. |
| `/2009-04-04/meta-data/facility` | GET | ✅ | ✅ | Facility code. |
| `/2009-04-04/meta-data/tags` | GET | ✅ | ✅ | Newline-separated tags. |
| `/2009-04-04/meta-data/public-ipv4` | GET | ✅ | ✅ | Public IPv4 address. |
| `/2009-04-04/meta-data/public-ipv6` | GET | ✅ | ✅ | Public IPv6 address. |
| `/2009-04-04/meta-data/local-ipv4` | GET | ✅ | ✅ | Private IPv4 address. |
| `/2009-04-04/meta-data/public-keys` | GET | ✅ | ✅ | Newline-separated SSH public keys. |
| `/2009-04-04/meta-data/operating-system/slug` | GET | ✅ | ✅ | OS slug identifier. |
| `/2009-04-04/meta-data/operating-system/distro` | GET | ✅ | ✅ | OS distribution name. |
| `/2009-04-04/meta-data/operating-system/version` | GET | ✅ | ✅ | OS version. |
| `/2009-04-04/meta-data/operating-system/image_tag` | GET | ✅ | ✅ | OS image tag. |
| `/2009-04-04/meta-data/operating-system/license_activation/state` | GET | ✅ | ✅ | License activation state. |
| `/tootles/` | GET | ✅ | ✅ | Instance-endpoint mirror of EC2 metadata (enabled via `--tootles-instance-endpoint`). Supports paths like `/tootles/instanceID/<id>/2009-04-04/...` |
| `/metadata` | GET | ✅ | ✅ | Legacy JSON endpoint returning Hardware storage/filesystem configuration. Used by the rootio action. |

### Web UI

The UI is served at a configurable URL prefix (default: `/`). All UI routes
support HTTPS and redirect HTTP → HTTPS when TLS is enabled.

| Route | Method | Description |
|-------|--------|-------------|
| `/` | GET | Dashboard (requires authentication or auto-login) |
| `/login` | GET | Login page |
| `/api/auth/login` | POST | Authentication endpoint (accepts kubeconfig) |
| `/api/auth/logout` | POST | Logout / session invalidation |
| `/hardware/` | GET | Hardware resource management |
| `/workflows/` | GET | Workflow resource management |
| `/templates/` | GET | Template resource management |
| `/bmc/` | GET | BMC (baseboard management controller) resource management |
| `/health` | GET | UI-specific health check (JSON) |
| `/ready` | GET | UI-specific readiness check (JSON) |
| `/css/`, `/js/`, `/artwork/`, `/fonts/` | GET | Static assets (24h cache) |
| `/favicon.ico`, `/favicon.svg` | GET | Favicon |

---

## gRPC Service

| Service | Port | Methods | Description |
|---------|------|---------|-------------|
| `github.com/tinkerbell/tinkerbell/pkg/proto.WorkflowService` | 42113 | `GetAction` | Agent requests the next workflow action to execute |
| | | `ReportActionStatus` | Agent reports completion/failure of an action |

The gRPC server supports:
- **TLS**: When `--tls-cert-file` and `--tls-key-file` are provided.
- **Server reflection**: Enabled for tooling like `grpcurl`.
- **OpenTelemetry**: Tracing via `otelgrpc` stats handler.

---

## Protocol Details

### DHCP (UDP :67)

Smee supports three DHCP modes:

| Mode | Description |
|------|-------------|
| `reservation` | Full DHCP server that assigns IPs from Hardware resources. Responds to DISCOVER, REQUEST, and RELEASE. |
| `proxy` | ProxyDHCP — does not assign IPs but provides PXE boot options (next-server, boot file) to supplement an existing DHCP server. |
| `auto-proxy` | Like `proxy`, but automatically determines whether to respond based on whether Hardware exists for the requesting MAC. |

Key DHCP options set:
- **Option 54** (Server Identifier): Tinkerbell's public IP
- **Option 66** (TFTP Server): Points to Tinkerbell's TFTP server
- **Option 67** (Bootfile Name): iPXE binary filename or HTTP URL
- **Option 7** (Log Server): Syslog IP for boot logging

### DHCPv6 (UDP :547)

Smee DHCPv6 is disabled by default and can be enabled with
`--dhcpv6-enabled=true`. It listens on `--dhcpv6-bind-addr` and
`--dhcpv6-bind-port`, defaulting to `[::]:547`. `--dhcpv6-bind-interface`
accepts either one interface or a comma-separated list, such as
`macvlan0,eth0`. When setting this through Helm, escape the comma and prefer
`--set-string`, for example
`--set-string deployment.envs.smee.dhcpv6BindInterface=macvlan0\,eth0`.

Smee supports four DHCPv6 modes:

| Mode | Description |
|------|-------------|
| `stateless` | Replies to matching Hardware records with stateless DHCPv6 configuration and optional boot data. IPv6 addressing comes from RA/SLAAC. |
| `auto-stateless` | Like `stateless`, but can answer unknown clients with default netboot data when the client identity can be determined. |
| `reservation` | Provides reserved IPv6 addresses from Hardware records using IA_NA. |
| `derived` | Uses Hardware IPv6 reservations when present, otherwise derives temporary boot-time IPv6 addresses from a configured direct pool or relay link-address prefix. |

Key DHCPv6 options set:
- **Option 2** (Server Identifier): Tinkerbell's DHCPv6 Server DUID
- **Option 3** (IA_NA): Reserved or derived IPv6 address in stateful modes
- **Option 23** (DNS Recursive Name Server): IPv6 DNS servers from Hardware DHCP configuration
- **Option 24** (Domain Search List): Domain search list from Hardware DHCP configuration
- **Option 32** (Information Refresh Time): Refresh interval for stateless replies
- **Option 56** (NTP Server): IPv6 NTP servers from Hardware DHCP configuration
- **Option 59** (Boot File URL): iPXE binary, script, or not-allowed URL, only when requested by the client

See [DHCP Boot Modes](./DHCP_BOOT_MODES.md) for DHCPv6 mode behavior,
addressing limitations, client identity requirements, and boot URL selection.

### TFTP (UDP :69)

Serves iPXE firmware binaries for initial PXE boot. Machines chain-load from
TFTP → iPXE binary → HTTP iPXE script → OS kernel/initrd over HTTP.

- Block size: 512 bytes (configurable)
- Single-port mode: enabled by default
- Timeout: 10 seconds per request

### Syslog (UDP :514)

Receives syslog messages from machines during the boot/provisioning process.
Messages are logged by Smee and can be used for debugging boot issues.

### SSH (TCP :2222)

SecondStar provides an SSH-to-serial-over-IPMI bridge. Operators SSH to
`<bmc-ip>@tinkerbell:2222` and are connected to the machine's serial console
via IPMI SOL (Serial Over LAN).

- Idle timeout: 15 minutes (configurable)
- Requires `ipmitool` at `/usr/sbin/ipmitool`

---

## TLS / HTTPS

When `--tls-cert-file` and `--tls-key-file` are provided:

1. The HTTPS server starts on port **7443** alongside HTTP on **7080**.
2. Select routes (metadata, UI, ISO) are served on both HTTP and HTTPS.
3. Some routes redirect HTTP → HTTPS automatically (308 Permanent Redirect).
4. The gRPC server (port 42113) also uses TLS.
5. Tink agents are informed of TLS via the iPXE kernel argument `tinkerbell_tls=true`.

### Disabling HTTP → HTTPS redirects

Pass `--disable-http-to-https-redirect` to keep all HTTP routes serving their
actual handlers instead of returning 308 redirects, even when TLS is configured.
This is useful when a load balancer or reverse proxy in front of Tinkerbell
terminates TLS and forwards plain HTTP to the server.

> **iPXE limitation:** iPXE only supports RSA TLS certificates. ECDSA
> certificates will cause iPXE binary/script downloads to fail over HTTPS.

---

## HTTP Middleware Stack

All HTTP/HTTPS requests pass through the following middleware (outermost first):

1. **SourceIP** — Captures the original TCP connection IP before XFF processing.
2. **XFF** (X-Forwarded-For) — Rewrites `RemoteAddr` based on trusted proxy headers.
3. **RequestMetrics** — Records `http_server_requests_total` and `http_server_request_duration_seconds`.
4. **Recovery** — Catches panics and returns 500.
5. **Logging** — Structured request/response logging with configurable verbosity per route.
6. **OpenTelemetry** — Distributed tracing spans for each request.

---

## Helm Chart Port Mapping

When deployed via Helm, the Kubernetes Service exposes:

| Service Port | Container Port | Protocol | Condition |
|-------------|----------------|----------|-----------|
| 7080 | 7080 | TCP | Always |
| 7443 | 7443 | TCP | TLS configured |
| 42113 | 42113 | TCP | `enableTinkServer` |
| 67 | 67 | UDP | `enableSmee` |
| 69 | 69 | UDP | `enableSmee` |
| 514 | 514 | UDP | `enableSmee` |
| 2222 | 2222 | TCP | `enableSecondstar` |
