#!/usr/bin/env bash
# Write a checksums file from the checksums a build job output, then check the
# files it names, which sit next to it.
#
# Artifacts carry binaries between jobs, but any job in the run can replace an
# artifact; only the job that built the binaries can set its outputs. So the
# jobs that publish, sign or attest binaries check them against those outputs,
# never against checksums that came with the download. The file comes out the
# same every time for the same outputs, so jobs that write it separately
# publish and attest the same bytes.
#
# Usage:
#   checksums.sh FILE SHA256SUMS...
#
# Arguments:
#   FILE        The checksums file to write, in the directory of the files to
#               check.
#   SHA256SUMS  The base64 of `sha256sum` output for files named without a
#               directory, as the build job outputs it. FILE holds the lines of
#               each, in the order given.

set -euo pipefail

if [[ $# -lt 2 ]]; then
	echo "usage: $0 FILE SHA256SUMS..." >&2
	exit 2
fi
file=$1
shift

sums=""
for b64 in "$@"; do
	if [[ -z "${b64}" ]]; then
		echo "::error::a build output no checksums"
		exit 1
	fi
	decoded=$(base64 --decode <<<"${b64}")
	while IFS= read -r line; do
		if [[ ! "${line}" =~ ^[0-9a-f]{64}\ \ [A-Za-z0-9._-]+$ ]]; then
			echo "::error::malformed checksum line from a build: '${line}'"
			exit 1
		fi
	done <<<"${decoded}"
	sums+="${decoded}"$'\n'
done

printf '%s' "${sums}" >"${file}"
cd "$(dirname "${file}")"
sha256sum --check --strict "$(basename "${file}")"
