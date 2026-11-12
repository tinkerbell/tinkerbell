#!/usr/bin/env bash
# Print the digest of a Helm chart pushed to an OCI registry, read from the
# output of `helm push`.
#
# Fails unless the output names exactly one push, to REPOSITORY, with a
# well-formed digest, so the digest printed is the chart that was pushed
# there.
#
# Usage:
#   helm-push-digest.sh PUSH_OUTPUT REPOSITORY
#
# Arguments:
#   PUSH_OUTPUT  File holding the stdout of `helm push`, which prints
#                "Pushed: <ref>:<version>" and "Digest: sha256:<hex>".
#   REPOSITORY   The chart repository the push must have gone to, without tag
#                (e.g. ghcr.io/tinkerbell/charts/tinkerbell).

set -euo pipefail

if [[ $# -ne 2 ]]; then
	echo "usage: $0 PUSH_OUTPUT REPOSITORY" >&2
	exit 2
fi
push_output=$1
expected_repository=$2

pushed=$(sed -n 's/^Pushed: //p' "${push_output}")
digest=$(sed -n 's/^Digest: //p' "${push_output}")
if [[ $(wc -l <<<"${pushed}") -ne 1 || -z "${pushed}" ]]; then
	echo "::error::expected one 'Pushed:' line in ${push_output}, got: ${pushed}" >&2
	exit 1
fi
if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
	echo "::error::expected one chart digest in ${push_output}, got: ${digest}" >&2
	exit 1
fi

# Strip the tag: everything after the last ':' that follows the last '/'. A
# registry port (host:5000/...) sits before a '/', so it is kept.
repository=${pushed%:*}
if [[ "${repository}" != */* || "${pushed##*/}" != *:* ]]; then
	echo "::error::cannot parse the chart reference: ${pushed}" >&2
	exit 1
fi
if [[ "${repository}" != "${expected_repository}" ]]; then
	echo "::error::the chart was pushed to ${repository}, expected ${expected_repository}" >&2
	exit 1
fi

echo "${digest}"
