#!/usr/bin/env bash
# Sign a multi-platform container image pushed by `docker buildx build --push`
# with cosign keyless signing.
#
# Signs the index and each platform manifest by digest, never by tag, so a
# reference pinned to either one verifies. BuildKit's attestation manifests
# are not signed on their own: the index signature covers them through their
# digests, and each extra signature costs several seconds of Sigstore and
# registry round trips.
#
# The signature carries the identity of whoever signs. In GitHub Actions with
# `id-token: write`, that is this repository's workflow. Without that OIDC
# token, cosign signs with whatever identity it finds instead (another CI's
# ambient token, or an interactive login) and pushes that signature to the
# registry, so the script refuses to run without it.
#
# Usage:
#   sign-image.sh IMAGE METADATA_FILE
#
# Arguments:
#   IMAGE          Image name without tag or digest (e.g. ghcr.io/tinkerbell/tinkerbell).
#   METADATA_FILE  The file `docker buildx build --metadata-file` wrote for the push.

set -euo pipefail

# cosign detects GitHub Actions through these two variables, which the runner
# sets only for jobs with `id-token: write`.
if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" || -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
	echo "$0: no GitHub Actions OIDC token (needs id-token: write); refusing to sign" >&2
	exit 1
fi

if [[ $# -ne 2 ]]; then
	echo "usage: $0 IMAGE METADATA_FILE" >&2
	exit 2
fi
image=$1
metadata_file=$2

digest=$(jq -r '."containerimage.digest"' "${metadata_file}")
if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
	echo "::error::no image digest in ${metadata_file}: ${digest}"
	exit 1
fi

index=$(docker buildx imagetools inspect --raw "${image}@${digest}")
# Not mapfile with process substitution: errexit would not see a jq failure.
platform_digests=$(jq -r '.manifests[] | select(.annotations["vnd.docker.reference.type"] != "attestation-manifest") | .digest' <<<"${index}")

refs=("${image}@${digest}")
for platform_digest in ${platform_digests}; do
	if [[ ! "${platform_digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
		echo "::error::unexpected platform manifest digest in ${image}@${digest}: ${platform_digest}"
		exit 1
	fi
	refs+=("${image}@${platform_digest}")
done
if [[ ${#refs[@]} -lt 2 ]]; then
	echo "::error::no platform manifests in ${image}@${digest}"
	exit 1
fi

cosign sign --yes "${refs[@]}"
