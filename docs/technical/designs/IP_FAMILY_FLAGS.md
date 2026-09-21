# IP Family Flags

Status: accepted, not yet implemented
Supersedes: `ipv6-flags.md`, an untracked scratch file at the repo root. It is not
committed, so removing it produces no diff.

## 1. Problem

IPv6 support arrived as a parallel set of flags alongside the IPv4 ones, with no stated
rule for which settings are family-scoped. Three problems follow from that:

1. **Two naming conventions are in play at once.** `--dhcpv6-*` uses an infix,
   `--ipxe-http-script-osie-url-v6` uses a suffix.
2. **Unsuffixed flags silently mean IPv4.** A missing `-v6` counterpart is
   indistinguishable from a deliberate omission. `--ipxe-http-script-extra-kernel-args`
   is shared by the iPXE script handler and both ISO routes, which quietly forces
   `tink_worker_image` and `insecure_registries` to be identical for IPv4 and IPv6
   machines.
3. **Listener bind addresses are single-valued.** Setting both `--public-ipv4` and
   `--public-ipv6` reads as dual-stack, but the runtime binds one socket per service
   and is effectively single-stack. Nothing validates or reports this.

## 2. Goals

- Family scope is explicit and uniform: a missing counterpart is a visible gap, not an
  implicit default.
- Single-stack IPv4, single-stack IPv6, and dual-stack are each clear and achievable
  in configuration, and documented as such.
- Existing deployments keep working across one upgrade cycle.

## 3. Non-goals

- Renaming Go struct fields in `smee.Config` and friends (`OSIEURLv6` stays as-is).
  Out of scope; revisit separately.
- Removing package-level `var`s from `cmd/tinkerbell/flag`, which conflict with
  `docs/DESIGN_PHILOSOPHY.md` §General.5. Out of scope; the mechanism in §5 reduces
  their count rather than adding to it.
- Per-family control of anything that is not family-scoped (see §4.3).

## 4. Naming rules

### 4.1 Suffixes

| Suffix | Meaning |
| --- | --- |
| *(none)* | applies to both families |
| `-v4` | applies only to IPv4 |
| `-v6` | applies only to IPv6 |

The `dhcpv6-` infix is retired. Family is always expressed as a trailing `-v4` / `-v6`.

### 4.2 Pairing

Every `-v4` flag has a `-v6` counterpart and vice versa, except for the entries listed
in §4.4. This is enforced by test (§8.3).

### 4.3 Flags that are not family-scoped

A flag takes no suffix when it configures behaviour that is identical for both families,
or a value that already accepts both. These keep their current names:

`--trusted-proxies`, `--ipxe-http-script-trusted-proxies` (both accept mixed-family
CIDRs); `--otel-*`, `--log-level`, `--backend*`, `--tls-*`,
`--disable-http-to-https-redirect`, `--enable-*`, `--version`;
`--dhcp-enabled` is *not* in this set (see §6);
`--ipxe-http-binary-enabled`, `--ipxe-http-script-enabled`,
`--ipxe-override-arch-mapping`, `--ipxe-embedded-script-patch`,
`--ipxe-binary-inject-mac-addr-format`, `--ipxe-http-script-kernel-name`,
`--ipxe-http-script-initrd-name`, `--ipxe-http-script-retries`,
`--ipxe-http-script-retry-delay`, `--ipxe-script-tink-server-use-tls`,
`--ipxe-script-tink-server-insecure-tls`; `--iso-enabled`, `--iso-upstream-url`,
`--iso-patch-magic-string`; `--syslog-enabled`, `--tftp-server-enabled`,
`--tftp-timeout`, `--tftp-block-size`, `--tftp-single-port`, `--tftp-asset-dir`;
`--pxe-http-*`; all `--rufio-*`, `--tink-controller-*`, `--tootles-*`, `--ui-*`,
`--etcd-*`, `--kubeapi-server-*`, and all `*-log-level` flags.

### 4.4 Intentionally single-family flags

These have no counterpart, by protocol design. They are the allow-list for the pairing
test in §8.3, and each must carry a usage string stating why.

| Flag | Reason |
| --- | --- |
| `--dhcp-ip-for-packet-v4` | DHCPv4 option 54 server identifier. DHCPv6 identifies by DUID. |
| `--dhcp-server-duid-v6` | DHCPv6 identifies by DUID, not address. No DHCPv4 equivalent. |
| `--dhcp-derived-direct-address-pool-v6` | Derived addressing is DHCPv6-only. |
| `--dhcp-derived-relay-address-prefix-v6` | Derived addressing is DHCPv6-only. |
| `--iso-static-ipam-enabled-v4` | Static IPAM in patched ISOs is IPv4-only. IPv6 uses SLAAC or DHCPv6. |

### 4.5 Environment variables

`ff` derives env vars as `TINKERBELL_` + `upper(name)` with `-` → `_`. Renaming a flag
therefore renames its env var. There is no separate env var mapping to maintain.

Example: `--dhcp-bind-addr-v6` → `TINKERBELL_DHCP_BIND_ADDR_V6`.

## 5. Mechanism

### 5.1 Family registration

`peterbourgon/ff v4.0.0-beta.1` `FlagConfig` has only `ShortName` and `LongName` — no
alias support, no hidden flags. Family is therefore a parameter of registration, not
part of the stored name.

Added to `cmd/tinkerbell/flag/flag.go`:

```go
// Family is an IP address family. Flags registered with one apply only to that
// family; flags registered without one apply to both.
type Family string

const (
	V4 Family = "v4"
	V6 Family = "v6"
)

// RegisterFamily registers f as "<name>-<family>", scoped to a single address family.
func (fs *Set) RegisterFamily(f Config, fam Family, fv flag.Value)
```

`RegisterFamily` appends the suffix to `f.Name` and annotates `f.Usage` with the family
before delegating to the existing `Register`. One `Config` declaration therefore serves
both families, which removes roughly 24 duplicated declarations from
`cmd/tinkerbell/flag/smee.go`. Call sites read as the pairing itself:

```go
fs.RegisterFamily(DHCPEnabled, V4, ffval.NewValueDefault(&sc.Config.DHCP.Enabled, ...))
fs.RegisterFamily(DHCPEnabled, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.Enabled, ...))
```

A missing counterpart is a missing line at the call site.

### 5.2 Deprecation shim

Retired names are rewritten to current names before parsing. Two functions in a new
`cmd/tinkerbell/flag/deprecated.go`:

```go
// RenameDeprecatedArgs rewrites retired flag names in args to their current names,
// returning the rewritten args and one message per rename applied.
func RenameDeprecatedArgs(args []string) ([]string, []string)

// RenameDeprecatedEnv copies values from retired TINKERBELL_ environment variables
// onto their current names, leaving an explicitly set current name alone. It returns
// one message per value copied.
func RenameDeprecatedEnv() ([]string, error)
```

Backed by an unexported `deprecatedNames() map[string]string` of retired name →
current name (54 entries when complete, §6).

Requirements:

- `RenameDeprecatedArgs` handles `--name value` and `--name=value`. `ff` only treats
  `--` as a long-name prefix, so single-dash forms need no handling.
- `RenameDeprecatedEnv` copies the value verbatim, including the empty string. `ff`
  distinguishes set-but-empty from unset, so skipping empties would change semantics.
- Both are called from `executeWithOutput` immediately before `cli.Parse`.
- Messages are buffered and emitted through `cliLog` after the logger is built — the
  log level is not known until after parsing.
- Message format: `<old> is deprecated and will be removed after <date>, use <new>`.
- The prefix `TINKERBELL` moves to an exported `flag.EnvVarPrefix` const, used by both
  the shim and `ff.WithEnvVarPrefix` in `cmd.go`, so it is not written twice.

**Removal target: 2027-09-01** (approximately one year). This affects only the warning
text and a doc comment on `deprecatedNames`; there is no runtime gate.

## 6. Flag mapping

Complete and authoritative. `NEW` means the flag does not exist today.

### 6.1 Globals — `cmd/tinkerbell/flag/global.go`

| Current | New |
| --- | --- |
| `--public-ipv4` | `--public-ip-v4` |
| `--public-ipv6` | `--public-ip-v6` |
| `--bind-address` | `--bind-address-v4` |
| NEW | `--bind-address-v6` |
| `--http-port` | `--http-port-v4` |
| NEW | `--http-port-v6` |
| `--https-port` | `--https-port-v4` |
| NEW | `--https-port-v6` |

### 6.2 Smee DHCP — `cmd/tinkerbell/flag/smee.go`

| Current | New |
| --- | --- |
| `--dhcp-enabled` | `--dhcp-enabled-v4` |
| `--dhcpv6-enabled` | `--dhcp-enabled-v6` |
| `--dhcp-mode` | `--dhcp-mode-v4` |
| `--dhcpv6-mode` | `--dhcp-mode-v6` |
| `--dhcp-enable-netboot-options` | `--dhcp-enable-netboot-options-v4` |
| `--dhcpv6-enable-netboot-options` | `--dhcp-enable-netboot-options-v6` |
| `--dhcp-bind-addr` | `--dhcp-bind-addr-v4` |
| `--dhcpv6-bind-addr` | `--dhcp-bind-addr-v6` |
| NEW | `--dhcp-bind-port-v4` |
| `--dhcpv6-bind-port` | `--dhcp-bind-port-v6` |
| `--dhcp-bind-interface` | `--dhcp-bind-interface-v4` |
| `--dhcpv6-bind-interface` | `--dhcp-bind-interface-v6` |
| `--dhcp-ip-for-packet` | `--dhcp-ip-for-packet-v4` |
| `--dhcpv6-server-duid` | `--dhcp-server-duid-v6` |
| NEW | `--dhcp-default-name-servers-v4` |
| `--dhcpv6-default-name-servers` | `--dhcp-default-name-servers-v6` |
| NEW | `--dhcp-default-domain-search-list-v4` |
| `--dhcpv6-default-domain-search-list` | `--dhcp-default-domain-search-list-v6` |
| `--dhcpv6-derived-direct-address-pool` | `--dhcp-derived-direct-address-pool-v6` |
| `--dhcpv6-derived-relay-address-prefix` | `--dhcp-derived-relay-address-prefix-v6` |
| `--dhcp-syslog-ip` | `--dhcp-syslog-ip-v4` |
| `--dhcpv6-syslog-ip` | `--dhcp-syslog-ip-v6` |
| `--dhcp-tftp-ip` | `--dhcp-tftp-ip-v4` |
| `--dhcpv6-tftp-ip` | `--dhcp-tftp-ip-v6` |
| `--dhcp-tftp-port` | `--dhcp-tftp-port-v4` |
| `--dhcpv6-tftp-port` | `--dhcp-tftp-port-v6` |
| `--dhcp-ipxe-http-script-prepend-mac` | `--dhcp-ipxe-http-script-prepend-mac-v4` |
| `--dhcpv6-ipxe-http-script-prepend-mac` | `--dhcp-ipxe-http-script-prepend-mac-v6` |

And, for each of `scheme`, `host`, `port`, `path`:

| Current | New |
| --- | --- |
| `--dhcp-ipxe-http-binary-<part>` | `--dhcp-ipxe-http-binary-<part>-v4` |
| `--dhcpv6-ipxe-http-binary-<part>` | `--dhcp-ipxe-http-binary-<part>-v6` |
| `--dhcp-ipxe-http-script-<part>` | `--dhcp-ipxe-http-script-<part>-v4` |
| `--dhcpv6-ipxe-http-script-<part>` | `--dhcp-ipxe-http-script-<part>-v6` |

### 6.3 iPXE, ISO, Syslog, TFTP — `cmd/tinkerbell/flag/smee.go`

| Current | New |
| --- | --- |
| `--ipxe-http-script-extra-kernel-args` | `--ipxe-http-script-extra-kernel-args-v4` |
| NEW | `--ipxe-http-script-extra-kernel-args-v6` |
| `--ipxe-http-script-osie-url` | `--ipxe-http-script-osie-url-v4` |
| `--ipxe-http-script-osie-url-v6` | unchanged |
| `--ipxe-script-syslog-fqdn` | `--ipxe-script-syslog-fqdn-v4` |
| `--ipxe-script-syslog-fqdn-v6` | unchanged |
| `--ipxe-script-tink-server-addr-port` | `--ipxe-script-tink-server-addr-port-v4` |
| `--ipxe-script-tink-server-addr-port-v6` | unchanged |
| `--iso-static-ipam-enabled` | `--iso-static-ipam-enabled-v4` |
| `--syslog-bind-addr` | `--syslog-bind-addr-v4` |
| NEW | `--syslog-bind-addr-v6` |
| `--syslog-bind-port` | `--syslog-bind-port-v4` |
| NEW | `--syslog-bind-port-v6` |
| `--tftp-server-bind-addr` | `--tftp-server-bind-addr-v4` |
| NEW | `--tftp-server-bind-addr-v6` |
| `--tftp-server-bind-port` | `--tftp-server-bind-port-v4` |
| NEW | `--tftp-server-bind-port-v6` |

### 6.4 Tink server — `cmd/tinkerbell/flag/tink_server.go`

| Current | New |
| --- | --- |
| `--tink-server-bind-addr` | `--tink-server-bind-addr-v4` |
| NEW | `--tink-server-bind-addr-v6` |
| `--tink-server-bind-port` | `--tink-server-bind-port-v4` |
| NEW | `--tink-server-bind-port-v6` |

### 6.5 SecondStar — `cmd/tinkerbell/flag/secondstar.go`

| Current | New |
| --- | --- |
| `--secondstar-port` | `--secondstar-port-v4` |
| NEW | `--secondstar-port-v6` |
| NEW | `--secondstar-bind-addr-v4` |
| NEW | `--secondstar-bind-addr-v6` |

### 6.6 Totals

54 renames, 16 new flags, 5 intentionally single-family.

## 7. Capability changes

Renaming alone does not make a `-v6` flag meaningful. Each new `-v6` listener flag
requires a second socket.

| Component | File | Today | Required |
| --- | --- | --- | --- |
| HTTP/HTTPS | `pkg/http/server/server.go` | one `BindAddr string`, one `BindPort`, one `HTTPSPort` | per-family bind address and ports; up to four listeners; skip a family whose bind address is unset |
| Syslog | `smee/smee.go` | one `netip.AddrPort` | two receivers |
| TFTP | `smee/smee.go` | one `binary.TFTP` | two instances |
| Tink gRPC | `tink/server/server.go` | one `BindAddrPort netip.AddrPort` | two listeners serving one `grpc.Server` |
| SSH | `secondstar/secondstar.go` | one `BindAddr`/`SSHPort` | two `gssh.Server` sharing handler and host key |

Two further capability changes are not about listeners:

- **Extra kernel args.** Split `IPXE.HTTPScriptServer.ExtraKernelArgs` into `...V4` and
  `...V6`, threaded into `ScriptHandler`, `ISOHandler`, and `ISOHandlerV6`. The
  deprecated `--ipxe-http-script-extra-kernel-args` must feed **both** to preserve
  current behaviour.
- **DHCPv4 DNS defaults.** `--dhcp-default-name-servers-v4` and
  `--dhcp-default-domain-search-list-v4` have no implementation today. Mirror the
  existing `v6.DNSDefaults` fallback into the v4 reservation handler
  (`smee/internal/dhcp/handler/reservation/option.go`), which currently reads
  nameservers only from Hardware.

## 8. Validation and reporting

### 8.1 Address family validation

Reject a value whose family contradicts its flag suffix. Today
`--dhcp-syslog-ip=2001:db8::1` is accepted silently. Applies to every `-v4` / `-v6`
address flag. Extends the existing `validatePublicAddressFamilies` in
`cmd/tinkerbell/defaults.go`.

### 8.2 Startup report

Log, per service, which families are actually listening and advertising, classified as
`ipv4-only`, `ipv6-only`, or `dual-stack`. This is what stops "both families set" from
reading as dual-stack when it is not.

### 8.3 Pairing test

Walk the registered flag set; assert every name ending in `-v4` has a `-v6` twin and
vice versa, with §4.4 as the allow-list. Test-local string handling; no production
helper.

## 9. Delivery

Four PRs, each on a branch built off the previous so they can be opened at once.
Counts are churn (added + deleted, as `git diff --shortstat` reports). No generated
code is involved — the refactor does not touch `api/` or CRDs.

| PR | Branch | Thesis | Go (non-test) | Go (test) | YAML + docs | Total |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| A | `ip-family-flags-shape` | Flag shape | 557 | 230 | 140 | 927 |
| B1 | `ip-family-flags-smee` | Smee dual-stack | 330 | 270 | 120 | 720 |
| B2 | `ip-family-flags-services` | HTTP, tink, secondstar dual-stack | 300 | 250 | 60 | 610 |
| C | `ip-family-flags-docs` | Guardrails + docs | 115 | 200 | 560 | 875 |
| | | | **1,302** | **950** | **880** | **~3,130** |

Budget: no PR exceeds 600 non-test lines of Go.

### PR A — flag shape

Zero behaviour change. Every rename in §6, plus the §5 mechanism.

Files: `cmd/tinkerbell/flag/flag.go` (new `Family`, `RegisterFamily`, `EnvVarPrefix`),
`cmd/tinkerbell/flag/deprecated.go` (new), `cmd/tinkerbell/flag/smee.go`,
`global.go`, `tink_server.go`, `secondstar.go`, `cmd/tinkerbell/cmd.go`,
`helm/tinkerbell/templates/deployment.yaml`.

New `-v6` flags whose listeners land in B1/B2 are **not** registered here. A `-v4` flag
without its twin is expected in this PR; §8.3 lands in C.

Also fixes, in `deployment.yaml`: `TINKERBELL_IPXE_SCRIPT_TRUSTED_PROXIES` →
`TINKERBELL_IPXE_HTTP_SCRIPT_TRUSTED_PROXIES`. The flag is
`--ipxe-http-script-trusted-proxies`, so the chart's value is silently ignored today.

Acceptance:
- Every name in §6 resolves; every retired name still works via args and env.
- A test asserts every `deprecatedNames` target exists in the registered flag set.
- No change to any service's runtime behaviour.

### PR B1 — Smee dual-stack

Syslog and TFTP second listeners, extra kernel args V4/V6 split, DHCPv4 DNS defaults,
`--dhcp-bind-port-v4`, and the corresponding new flags from §6.

Files: `smee/smee.go`, `smee/internal/dhcp/handler/reservation/option.go`,
`smee/internal/ipxe/script/`, `smee/internal/iso/`, `cmd/tinkerbell/flag/smee.go`,
`deprecated.go`, `helm/tinkerbell/templates/deployment.yaml`.

Acceptance:
- Smee binds syslog and TFTP on both families when both are configured, and on exactly
  one when only one is.
- `--ipxe-http-script-extra-kernel-args` (deprecated) still applies to both families.

### PR B2 — HTTP, tink server, secondstar dual-stack

Files: `pkg/http/server/server.go`, `tink/server/server.go`,
`secondstar/secondstar.go`, `cmd/tinkerbell/http.go`, `cmd.go`, `defaults.go`,
`cmd/tinkerbell/flag/global.go`, `tink_server.go`, `secondstar.go`, `deprecated.go`,
`helm/tinkerbell/templates/deployment.yaml`.

Watch item: `--bind-address` is read by `http.go`, `ts.Convert`, `ssc.Config.BindAddr`,
and `s.Convert` for the syslog/TFTP fallback. If the precedence rework pushes this PR
over budget, split the fallback change out.

Acceptance:
- Each of the four HTTP listeners (v4/v6 × HTTP/HTTPS) binds only when its family is
  configured.
- Single-stack IPv6 works end to end with no IPv4 address configured anywhere.

### PR C — guardrails and docs

§8.1 validation, §8.2 startup report, §8.3 pairing test, and documentation.

Files: `cmd/tinkerbell/defaults.go`, `cmd.go`, new
`docs/technical/IP_FAMILY_CONFIGURATION.md` (single-stack v4 / single-stack v6 /
dual-stack guide), `docs/technical/PORTS_AND_ENDPOINTS.md`,
`docs/technical/DHCP_BOOT_MODES.md`, `docs/technical/TLS_TERMINATION.md`,
`docs/technical/L3_ISO_SUPPORT.md`, `helm/tinkerbell/README.md`,
`helm/tinkerbell/values.yaml`.

Acceptance:
- Every documented example is one of the three topologies, labelled as such.
- §8.3 passes with §4.4 as the only allow-list.

## 10. Verification

```sh
# Full inventory of registered flag names, for diffing against §6.
go run ./cmd/tinkerbell --help 2>&1 | grep -oE '^\s+--[a-z0-9-]+' | tr -d ' '

# Pairing and deprecation.
go test ./cmd/tinkerbell/...

# Per-PR churn against the budget in §9.
git diff --shortstat <base>..HEAD -- '*.go' ':!*_test.go'
```

## 11. References

- `docs/DESIGN_PHILOSOPHY.md`
- <https://go.dev/doc/effective_go>
- <https://go.dev/wiki/CodeReviewComments>
- <https://google.github.io/styleguide/go/decisions#naming>
- <https://dave.cheney.net/2016/08/20/solid-go-design>
