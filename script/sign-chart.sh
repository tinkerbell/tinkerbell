#!/usr/bin/env bash
# Sign a Helm chart pushed to an OCI registry with cosign keyless signing.
#
# Reads the chart reference and digest from the output of `helm push`, so the
# signature goes to exactly what was pushed, by digest, never by tag.
#
# The signature carries the identity of whoever signs. In GitHub Actions with
# `id-token: write`, that is this repository's workflow. Without that OIDC
# token, cosign signs with whatever identity it finds instead (another CI's
# ambient token, or an interactive login) and pushes that signature to the
# registry, so the script refuses to run without it.
#
# Usage:
#   sign-chart.sh PUSH_OUTPUT REPOSITORY
#
# Arguments:
#   PUSH_OUTPUT  File holding the stdout of `helm push`, which prints
#                "Pushed: <ref>:<version>" and "Digest: sha256:<hex>".
#   REPOSITORY   The chart repository the push must have gone to, without tag
#                (e.g. ghcr.io/tinkerbell/charts/tinkerbell).

set -euo pipefail

# cosign detects GitHub Actions through these two variables, which the runner
# sets only for jobs with `id-token: write`.
if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" || -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
	echo "$0: no GitHub Actions OIDC token (needs id-token: write); refusing to sign" >&2
	exit 1
fi

if [[ $# -ne 2 ]]; then
	echo "usage: $0 PUSH_OUTPUT REPOSITORY" >&2
	exit 2
fi
push_output=$1
expected_repository=$2

pushed=$(sed -n 's/^Pushed: //p' "${push_output}")
digest=$(sed -n 's/^Digest: //p' "${push_output}")
if [[ $(wc -l <<<"${pushed}") -ne 1 || -z "${pushed}" ]]; then
	echo "::error::expected one 'Pushed:' line in ${push_output}, got: ${pushed}"
	exit 1
fi
if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
	echo "::error::expected one chart digest in ${push_output}, got: ${digest}"
	exit 1
fi

# Strip the tag: everything after the last ':' that follows the last '/'. A
# registry port (host:5000/...) sits before a '/', so it is kept.
repository=${pushed%:*}
if [[ "${repository}" != */* || "${pushed##*/}" != *:* ]]; then
	echo "::error::cannot parse the chart reference: ${pushed}"
	exit 1
fi
if [[ "${repository}" != "${expected_repository}" ]]; then
	echo "::error::the chart was pushed to ${repository}, expected ${expected_repository}"
	exit 1
fi

cosign sign --yes "${repository}@${digest}"
