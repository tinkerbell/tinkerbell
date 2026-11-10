#!/usr/bin/env bash
# Sign a Helm chart pushed to an OCI registry with cosign keyless signing.
#
# Signs the chart by digest, never by tag.
#
# The signature carries the identity of whoever signs. In GitHub Actions with
# `id-token: write`, that is this repository's workflow. Without that OIDC
# token, cosign signs with whatever identity it finds instead (another CI's
# ambient token, or an interactive login) and pushes that signature to the
# registry, so the script refuses to run without it.
#
# Usage:
#   sign-chart.sh CHART DIGEST
#
# Arguments:
#   CHART   Chart repository without tag or digest
#           (e.g. ghcr.io/tinkerbell/charts/tinkerbell).
#   DIGEST  Digest of the pushed chart, from the output of `helm push`
#           (script/helm-push-digest.sh).

set -euo pipefail

# cosign detects GitHub Actions through these two variables, which the runner
# sets only for jobs with `id-token: write`.
if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" || -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
	echo "$0: no GitHub Actions OIDC token (needs id-token: write); refusing to sign" >&2
	exit 1
fi

if [[ $# -ne 2 ]]; then
	echo "usage: $0 CHART DIGEST" >&2
	exit 2
fi
chart=$1
digest=$2

if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
	echo "::error::malformed chart digest for ${chart}: '${digest}'"
	exit 1
fi

cosign sign --yes "${chart}@${digest}"
