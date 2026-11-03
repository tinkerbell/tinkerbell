#!/usr/bin/env bash
# Write or check checksums-embedded.txt, the checksums of the embedded release
# binaries.
#
# The checksums come from build-embedded-binaries' job outputs, passed in
# SHA256_AMD64 and SHA256_ARM64, never from the downloaded files: any job in
# the run can replace an artifact, but only build-embedded-binaries can set
# its outputs. So the release jobs attest and publish only what that job
# built.
#
# Usage:
#   release-checksums.sh write DIR
#   release-checksums.sh check DIR
#
#   write  Write DIR/checksums-embedded.txt, then check DIR's binaries.
#   check  Check that DIR/checksums-embedded.txt holds those checksums, then
#          check DIR's binaries.

set -euo pipefail

if [[ $# -ne 2 || ( "$1" != write && "$1" != check ) ]]; then
	echo "usage: $0 write|check DIR" >&2
	exit 2
fi
mode=$1
dir=$2

for sha in "${SHA256_AMD64:-}" "${SHA256_ARM64:-}"; do
	if [[ ! "${sha}" =~ ^[0-9a-f]{64}$ ]]; then
		echo "::error::missing or malformed binary checksum from build-embedded-binaries: '${sha}'"
		exit 1
	fi
done

checksums=$(printf '%s  %s\n' \
	"${SHA256_AMD64}" tinkerbell-embedded-linux-amd64 \
	"${SHA256_ARM64}" tinkerbell-embedded-linux-arm64)

if [[ "${mode}" == write ]]; then
	echo "${checksums}" >"${dir}/checksums-embedded.txt"
elif [[ "$(cat "${dir}/checksums-embedded.txt")" != "${checksums}" ]]; then
	echo "::error::${dir}/checksums-embedded.txt does not hold the checksums build-embedded-binaries output"
	exit 1
fi

cd "${dir}"
sha256sum --check --strict checksums-embedded.txt
