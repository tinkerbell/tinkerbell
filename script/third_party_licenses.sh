#!/usr/bin/env bash
# Generate a per-arch THIRD_PARTY_LICENSES bundle for the tinkerbell and
# tink-agent binaries.
#
# Stages every LICENSE / NOTICE file for the modules compiled into the
# binaries (via `go-licenses save`), then concatenates them into a single
# human-readable bundle prefixed with the project header text.
#
# Usage:
#   third_party_licenses.sh ARCH OUT STAGE_DIR GO_LICENSES HEADER
#
# Arguments:
#   ARCH         GOARCH for the build whose bundle we're producing (amd64|arm64).
#   OUT          Path to the bundle file to write (e.g.
#                out/THIRD_PARTY_LICENSES-linux-amd64).
#   STAGE_DIR    Scratch directory for `go-licenses save` output.
#   GO_LICENSES  Path to the go-licenses binary.
#   HEADER       Path to the bundle preamble text file.
#
# Environment:
#   GOFLAGS  Optional. Preserved as-is and forwarded to `go-licenses` so
#            caller- or CI-supplied flags (e.g. -mod=readonly, -trimpath) are
#            honored when resolving the module graph.
#   GO_TAGS  Optional comma-separated build tags. Appended to GOFLAGS as
#            `-tags=<GO_TAGS>` so arch- or tag-gated dependencies match the
#            shipped binary. Mirrors the cross-compile / cross-compile-agent
#            targets.

set -euo pipefail

if [[ $# -ne 5 ]]; then
	echo "usage: $0 ARCH OUT STAGE_DIR GO_LICENSES HEADER" >&2
	exit 2
fi

ARCH=$1
OUT=$2
STAGE_DIR=$3
GO_LICENSES=$4
HEADER=$5

# Resolve OUT to an absolute path before we `cd` into STAGE_DIR below.
mkdir -p "$(dirname "$OUT")"
OUT_ABS=$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")

GOFLAGS="${GOFLAGS:-}"
if [[ -n "${GO_TAGS:-}" ]]; then
	# Append -tags so any caller-supplied GOFLAGS (e.g. -mod=readonly,
	# -trimpath) are preserved.
	GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=${GO_TAGS}"
fi

rm -rf "$STAGE_DIR" "$OUT_ABS"

# Skip the main module so its LICENSE (separately copied as
# /usr/share/doc/tinkerbell/LICENSE in the images) is not duplicated in the
# third-party bundle.
#
# GOOS / GOARCH / GOFLAGS are consumed indirectly: go-licenses uses
# golang.org/x/tools/go/packages to walk the import graph, which shells out
# to the Go toolchain (`go list`). That respects GOOS/GOARCH/GOFLAGS for
# build-tag filtering (`//go:build linux`, `_arm64.go`, `-tags=…`), so these
# are what make the bundle reflect the actual per-arch shipped binary
# instead of the host's. GOFLAGS is exported via the prefix because the
# `GOFLAGS="${GOFLAGS:-}"` assignment above is local-only (not exported).
#
# GOTOOLCHAIN=local pins `go list` to the host-installed toolchain. Without
# this, Go's auto-toolchain feature downloads the exact version named in
# go.mod's `go` directive into the module cache and uses that as GOROOT.
# go-licenses' stdlib detection does not recognize packages served from a
# downloaded-toolchain GOROOT, so every stdlib import (`context`, `fmt`,
# `crypto/*`, …) trips the "Non go modules projects are no longer supported"
# error and the run fails. See google/go-licenses#128. The host toolchain
# must be >= the go.mod directive (already a requirement for `go build`).
GOOS=linux GOARCH="$ARCH" GOFLAGS="$GOFLAGS" GOTOOLCHAIN=local \
	"$GO_LICENSES" save ./cmd/tinkerbell ./cmd/agent \
		--save_path="$STAGE_DIR" --force \
		--ignore github.com/tinkerbell/tinkerbell

cat "$HEADER" > "$OUT_ABS"

cd "$STAGE_DIR"
find . -type f | LC_ALL=C sort | while read -r f; do
	module=$(dirname "${f#./}")
	name=$(basename "$f")
	{
		echo ""
		echo "================================================================================"
		echo "Module: $module"
		echo "File:   $name"
		echo "================================================================================"
		echo ""
		cat "$f"
	} >> "$OUT_ABS"
done
