#!/usr/bin/env bash
# Verify, write or check checksums-embedded.txt, the checksums of the release
# assets: the embedded binaries and the files generated from them, such as
# SBOMs.
#
# Any job in the run can replace an artifact, but only the job that sets an
# output can set it. So the binaries' checksums come from
# build-embedded-binaries' outputs, passed in SHA256_AMD64 and SHA256_ARM64,
# and the checksums file comes from prepare-release's output, passed
# base64-encoded in CHECKSUMS, never from the downloaded files. The release
# jobs then attest and publish only what those jobs produced.
#
# Usage:
#   release-checksums.sh verify DIR
#   release-checksums.sh write DIR [FILE...]
#   release-checksums.sh check DIR
#
#   verify  Check DIR's binaries against SHA256_AMD64 and SHA256_ARM64. Run it
#           before any tool reads the binaries.
#   write   Verify, then write DIR/checksums-embedded.txt for the binaries and
#           each FILE in DIR. Run it in the job that generated the FILEs.
#   check   Check that DIR/checksums-embedded.txt is CHECKSUMS, byte for byte,
#           and holds SHA256_AMD64 and SHA256_ARM64, then check every file it
#           lists.

set -euo pipefail

if [[ $# -lt 2 ]]; then
	echo "usage: $0 verify DIR | write DIR [FILE...] | check DIR" >&2
	exit 2
fi
mode=$1
dir=$2
shift 2
case "${mode}" in
verify | check)
	if [[ $# -ne 0 ]]; then
		echo "usage: $0 ${mode} DIR" >&2
		exit 2
	fi
	;;
write) ;;
*)
	echo "usage: $0 verify DIR | write DIR [FILE...] | check DIR" >&2
	exit 2
	;;
esac

for sha in "${SHA256_AMD64:-}" "${SHA256_ARM64:-}"; do
	if [[ ! "${sha}" =~ ^[0-9a-f]{64}$ ]]; then
		echo "::error::missing or malformed binary checksum from build-embedded-binaries: '${sha}'"
		exit 1
	fi
done

binaries=$(printf '%s  %s\n' \
	"${SHA256_AMD64}" tinkerbell-embedded-linux-amd64 \
	"${SHA256_ARM64}" tinkerbell-embedded-linux-arm64)

cd "${dir}"

case "${mode}" in
verify)
	sha256sum --check --strict <<<"${binaries}"
	;;
write)
	sha256sum --check --strict <<<"${binaries}"
	{
		echo "${binaries}"
		if [[ $# -gt 0 ]]; then
			sha256sum -- "$@"
		fi
	} >checksums-embedded.txt
	;;
check)
	if [[ -z "${CHECKSUMS:-}" ]]; then
		echo "::error::no checksums from prepare-release"
		exit 1
	fi
	expected=$(mktemp)
	trap 'rm -f "${expected}"' EXIT
	base64 --decode <<<"${CHECKSUMS}" >"${expected}"
	# Compare bytes: command substitution would ignore trailing newlines.
	if ! cmp --silent "${expected}" checksums-embedded.txt; then
		echo "::error::${dir}/checksums-embedded.txt is not the file prepare-release wrote: another job replaced the artifact"
		exit 1
	fi
	if [[ "$(head -n 2 checksums-embedded.txt)" != "${binaries}" ]]; then
		echo "::error::prepare-release's checksums do not hold the checksums build-embedded-binaries output"
		exit 1
	fi
	sha256sum --check --strict checksums-embedded.txt
	;;
esac
