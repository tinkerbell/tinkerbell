#!/usr/bin/env bash
# Decide, once per CI run, the version the publishing jobs publish and whether
# they publish.
#
# The version comes from the commit's VCS stamp (script/version), so a main
# build of a commit that already carries a release tag computes that release's
# version. The tag build owns that version and signs it with the release
# identity. A main build that pushed it too would move the release's tags to
# artifacts signed as main, which then fail release verification. Such a main
# build publishes nothing.
#
# The publishing jobs take the version from here instead of computing it from
# their own checkouts, so a tag pushed while the run is going cannot change
# what they publish.
#
# A tag build fails unless the version is the tag that triggered it: a commit
# carrying another version tag would otherwise publish that version, signed as
# this tag.
#
# Writes version=<version> and publish=true or publish=false to $GITHUB_OUTPUT.

set -euo pipefail

version=$(go run --buildvcs=true ./script/version/)
echo "version=${version}" >>"${GITHUB_OUTPUT}"

case "${GITHUB_REF}" in
refs/tags/*)
	if [[ "${version}" != "${GITHUB_REF_NAME}" ]]; then
		tags=$(git tag --points-at HEAD 'v*' | sort -V | paste -sd ' ' -)
		echo "::error::${GITHUB_REF_NAME} builds as version ${version} (tags on this commit: ${tags}); a release tag must be the highest semver tag on its commit"
		exit 1
	fi
	;;
refs/heads/main)
	# Not a pipe into grep -q: pipefail would fail on git tag's SIGPIPE.
	tags=$(git tag --points-at HEAD)
	if grep -qxF -- "${version}" <<<"${tags}"; then
		echo "::notice::${version} is tagged on this commit: its tag build publishes it, so this main build publishes nothing"
		echo "publish=false" >>"${GITHUB_OUTPUT}"
		exit 0
	fi
	;;
esac
echo "publish=true" >>"${GITHUB_OUTPUT}"
