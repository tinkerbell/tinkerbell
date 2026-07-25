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
| **69** | UDP | Smee TFTP | Serves iPXE firmware binaries, and (optionally) PXELinux configs, Raspberry Pi netboot firmware, and arbitrary disk assets to PXE-booting machines | `--enable-smee=false` |
| **514** | UDP | Smee Syslog | Collects boot-time syslog messages from provisioning machines | `--enable-smee=false` |
| **2222** | TCP (SSH) | SecondStar | SSH-to-serial bridge for out-of-band hardware management via BMC | `--enable-secondstar=false` |

> **Note:** The HTTP and HTTPS ports are configurable via `--http-port` and
> `--https-port`. The gRPC port is configurable via `--tink-server-bind-port`.
> The TFTP server can serve extra on-disk assets from a directory configured
> via `--tftp-asset-dir` (env `TINKERBELL_TFTP_ASSET_DIR`); it is disabled when
> empty (the default). The same PXELinux configs and asset directory can also
> be served over the consolidated HTTP server ("PXE over HTTP", for u-boot's
> pxe-over-http/wget clients) by enabling `--pxe-http-enabled`; the URL path
> prefix it mounts under is set with `--pxe-http-path-prefix` (default
> `/tftp/`).

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
| `/ipxe/binary/` | GET, HEAD | | | Serves architecture-specific iPXE firmware binaries (e.g. `snp.efi`, `undionly.kpxe`) from the embedded file set. DHCP option 67 points machines here. |
| `/ipxe/script/` | GET | | | Serves auto-generated iPXE boot scripts. Supports MAC-address injection in the URL path (e.g. `/ipxe/script/aa:bb:cc:dd:ee:ff/auto.ipxe`). |
| `/iso/` | GET | ✅ | | Serves dynamically-patched ISO images with per-machine kernel parameters baked in. Enabled via `--smee-iso-enabled`. |

### PXE over HTTP (Smee)

When `--pxe-http-enabled` is set, Smee mounts a handler on the consolidated
HTTP server that serves the **same routes as the TFTP server** over HTTP, for
clients that netboot via HTTP (e.g. u-boot's pxe-over-http, which uses `wget`)
using the same request-path shapes as TFTP. It is disabled by default.

The handler reuses the TFTP route abstraction, but with a reduced route set:
only the PXELinux and disk-asset routes are enabled (the embedded-iPXE route is
omitted because iPXE binaries are already served at `/ipxe/binary/`, and the
Raspberry Pi EEPROM route is out of scope for u-boot). All paths below are
relative to the configured prefix (`--pxe-http-path-prefix`, default `/tftp/`).

| Route | Method | HTTPS | Redirect | Description |
|-------|--------|-------|----------|-------------|
| `{prefix}/pxelinux.cfg/01-<MAC>` | GET, HEAD | | | Serves the on-the-fly `pxelinux.cfg` (extlinux format) from Hardware `spec.netboot.pxelinux.config`, looked up by the dashed MAC in the path (either case accepted). |
| `{prefix}/<path>` | GET, HEAD | | | Serves an arbitrary file from the TFTP asset directory (`--tftp-asset-dir`). Path traversal outside the directory is rejected. Returns 404 when the file does not exist or no asset directory is configured. |

`HEAD` requests return the `Content-Length` header (set for seekable payloads)
without a body. Non-`GET`/`HEAD` methods return `405 Method Not Allowed`.

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

## TFTP Endpoints

Smee's TFTP server (UDP `:69`) serves boot files to machines that PXE-boot over
TFTP. Each incoming request is offered to a list of routes **in order**. A route
inspects the requested path and either **claims** the request (serves it, and
the search stops) or **declines** it (the next route is tried). The routes are
not alternative filenames that get retried — the client always asks for one
name, and each route just decides whether that name is "its" kind of request. If
no route claims it, the server returns a "file unknown" (not-found) error.

Most routes are mutually exclusive by path shape, so ordering only decides
precedence in the rare case where more than one route could claim the same path
(first match wins). For example, a request for `snp.efi` is claimed by the
embedded-iPXE route (it is a compiled-in name); a request for
`pxelinux.cfg/01-AA-BB-CC-DD-EE-FF` is claimed by the PXELinux route; anything
else falls through to the disk-asset route if a file of that name exists.

| Requested Path (matched by route) | Served From | Hardware Lookup | Description |
|-----------------------------------|-------------|-----------------|-------------|
| `<name>` matches the basename of an embedded iPXE file (e.g. `snp.efi`, `undionly.kpxe`) | Embedded (compiled-in) iPXE binaries | None | Serves the iPXE bootloader, optionally patched with the embedded-script patch. This is the default TFTP boot path. |
| `pxelinux.cfg/01-<MAC>` | Hardware `spec.netboot.pxelinux.config`, else a static asset-dir file | By MAC (parsed from path; either case) | Serves an on-the-fly `pxelinux.cfg` (extlinux format) for u-boot PXELinux booting. |
| `<SerialNum>/config.txt` | Hardware `spec.netboot.rpi.configTxt`, else a static asset-dir file | By client IP | Raspberry Pi EEPROM netboot `config.txt`. |
| `<SerialNum>/cmdline.txt` | Hardware `spec.netboot.osie.kernelParams` (space-joined), else a static asset-dir file | By client IP | Raspberry Pi EEPROM netboot `cmdline.txt`. |
| `<SerialNum>/<file>` (any other file under the serial prefix) | Asset directory, at `<firmwarePath>/<file>` (see below) | By client IP | Raspberry Pi firmware/binaries (e.g. `start4.elf`, `.dtb`). |
| `<path>` (any remaining path) | Asset directory (`--tftp-asset-dir`) | None | Fallback: serves an arbitrary file from the asset directory. Path traversal outside the directory is rejected. Returns not-found when the file is absent or no asset directory is configured. |

**Notes:**

- The Raspberry Pi and disk-asset routes require an asset directory
  (`--tftp-asset-dir`, env `TINKERBELL_TFTP_ASSET_DIR`); both are disabled when
  it is empty (the default). The Raspberry Pi routes additionally require the
  matched Hardware to set `spec.netboot.rpi.serialNum` and
  `spec.netboot.rpi.firmwarePath`.
- The Raspberry Pi serial number is not derived from the MAC: the Hardware is
  found by client IP, and its serial number is matched against the request
  path's prefix.
- For Raspberry Pi firmware files, the serial-number prefix in the requested
  path is mapped to a location **inside the asset directory** before the file is
  read from local disk: a request for `<SerialNum>/start4.elf` is served from
  the file `<assetDir>/<firmwarePath>/start4.elf`. This is a purely internal
  disk-path mapping performed by Smee; the TFTP client is **not** redirected and
  never sees `<firmwarePath>` — it requested `<SerialNum>/start4.elf` and
  receives those bytes in response.
- Clients may append a traceparent to the requested filename; it is stripped
  before route matching.
- The PXELinux and disk-asset routes can additionally be exposed over the HTTP
  server; see [PXE over HTTP (Smee)](#pxe-over-http-smee).

These routes rely on the following Hardware fields:

A normal iPXE boot needs **none** of these fields — it uses the compiled-in iPXE
binaries with no Hardware configuration. They apply only when you opt into
PXELinux or Raspberry Pi netboot:

| Hardware field | Required? | Effect when unset |
|---|---|---|
| `spec.netboot.rpi.serialNum` | Required to enable the RPi route (gates it together with `firmwarePath`) | RPi route is skipped for this machine |
| `spec.netboot.rpi.firmwarePath` | Required to enable the RPi route | RPi route is skipped for this machine |
| `spec.netboot.pxelinux.config` | Optional | `pxelinux.cfg/01-<MAC>` is not generated; the request falls through (see precedence) |
| `spec.netboot.rpi.configTxt` | Optional | `<SerialNum>/config.txt` is not served inline; the request falls through |
| `spec.netboot.osie.kernelParams` | Optional | `<SerialNum>/cmdline.txt` is not served inline; the request falls through |

`serialNum` is the full serial number or its last 8 characters. `firmwarePath`
is the asset-directory subpath that the serial prefix maps to on disk (see the
Raspberry Pi firmware note above); it is internal to Smee, not a client-visible
redirect.

**Precedence.** There are **no built-in default templates** — when a Hardware
field is empty its route does not synthesize a default; it declines, and the
request is offered to the next route. So for the PXELinux / `config.txt` /
`cmdline.txt` paths the effective order is:

1. The Hardware field, served inline, if set.
2. Otherwise, a static file of the same requested name under `--tftp-asset-dir`
   (served by the disk-asset route, only if an asset directory is configured).
3. Otherwise, not-found.

All other Raspberry Pi firmware/binaries, and any other requested file, come
solely from the disk-asset route (`--tftp-asset-dir`); they have no
Hardware-field source.

**`kernelParams` has a second, separate use.** Besides the Raspberry Pi
`cmdline.txt` above, `spec.netboot.osie.kernelParams` is also injected into the
generated **iPXE boot script** (a different code path from TFTP serving). There
the precedence is additive rather than fall-through: the global
`--ipxe-http-script-extra-kernel-args` values are applied first, then the
per-Hardware `osie.kernelParams` are appended, so machine-specific values win on
duplicate keys (the Linux kernel command line is last-wins).

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

### TFTP (UDP :69)

Serves boot files for initial PXE boot; see the [TFTP Endpoints](#tftp-endpoints)
section for the full list of served paths and their sources. In the common iPXE
case, machines chain-load from TFTP → iPXE binary → HTTP iPXE script → OS
kernel/initrd over HTTP.

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
