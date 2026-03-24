#!/usr/bin/env bash

# Re-tag existing OCI images built from the current commit with a release
# version (and `latest`). Intended to be invoked from CI on a release tag
# push, but safe to run locally provided `crane` is on PATH and already
# authenticated against the target registry.
#
# Inputs (environment variables):
#   TAG_VERSION  (required) - the release tag to apply, e.g. v0.20.0.
#   SHORT_SHA    (optional) - the short commit SHA suffix to match against
#                             existing image tags. Defaults to the first 8
#                             chars of HEAD. This must be a deterministic
#                             8-char prefix of the full SHA (not
#                             `git rev-parse --short=8`, which is a
#                             *minimum* width and may return more chars on
#                             prefix collisions) so it matches the suffix
#                             baked into mainline image tags by
#                             `pkg/build/build.go`.
#   IMAGES       (optional) - space-separated list of image repositories to
#                             re-tag. Defaults to the canonical Tinkerbell
#                             release image set.
#
# For each image, this script looks up an existing tag ending in
# `-${SHORT_SHA}` and adds both ${TAG_VERSION} and `latest` to it. Fails
# fast if zero or more than one matching tag is found.

set -o errexit -o nounset -o pipefail

if [ -z "${TAG_VERSION:-}" ]; then
	echo "ERROR: TAG_VERSION must be set (e.g. TAG_VERSION=v0.20.0)" >&2
	exit 1
fi

# Guard against accidental tag pushes (e.g. `vtest`) silently moving the
# `latest` tag in production. Accept vX.Y.Z and vX.Y.Z-<pre-release>.
if ! [[ "${TAG_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.+-]+)?$ ]]; then
	echo "ERROR: TAG_VERSION ${TAG_VERSION} is not a valid release version (expected vX.Y.Z or vX.Y.Z-<pre-release>)" >&2
	exit 1
fi

IMAGES="${IMAGES:-ghcr.io/tinkerbell/tinkerbell ghcr.io/tinkerbell/tink-agent}"

# Use a deterministic 8-char prefix of the full SHA. `git rev-parse --short=8`
# is a *minimum* width and may return more characters when the prefix is
# ambiguous, which would not match the suffix on mainline image tags (those
# are sliced to exactly 8 chars by `pkg/build/build.go`).
SHORT_SHA="${SHORT_SHA:-$(git rev-parse HEAD | cut -c1-8)}"
echo "Release version: ${TAG_VERSION}"
echo "Commit short SHA: ${SHORT_SHA}"

for IMAGE in ${IMAGES}; do
	echo "Looking up existing tag for ${IMAGE} with SHA ${SHORT_SHA}..."
	mapfile -t MATCHING_TAGS < <(crane ls "${IMAGE}" | grep "\-${SHORT_SHA}$" || true)
	if [ "${#MATCHING_TAGS[@]}" -eq 0 ]; then
		echo "ERROR: No tag ending in -${SHORT_SHA} found for ${IMAGE}"
		echo "This commit may not have been built on main. Available tags:"
		crane ls "${IMAGE}" | tail -20
		exit 1
	elif [ "${#MATCHING_TAGS[@]}" -gt 1 ]; then
		echo "ERROR: Multiple tags ending in -${SHORT_SHA} found for ${IMAGE}:"
		printf '  %s\n' "${MATCHING_TAGS[@]}"
		echo "Refusing to guess which tag to re-tag. Please clean up duplicate tags and retry."
		exit 1
	fi
	EXISTING_TAG="${MATCHING_TAGS[0]}"
	echo "Found existing tag: ${EXISTING_TAG}"
	echo "Adding tag ${TAG_VERSION}..."
	crane tag "${IMAGE}:${EXISTING_TAG}" "${TAG_VERSION}"
	echo "Adding tag latest..."
	crane tag "${IMAGE}:${EXISTING_TAG}" "latest"
	echo "Done re-tagging ${IMAGE}"
done
