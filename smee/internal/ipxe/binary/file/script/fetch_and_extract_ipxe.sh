#!/usr/bin/env bash

set -euo pipefail

# download_ipxe_repo will download a source archive from github.
function download_ipxe_repo() {
	local sha_or_tag="$1"
	local archive_name="$2"
	if [[ ! -f "${archive_name}" ]]; then
		echo "downloading"
		curl -fLo "${archive_name}.tmp" https://github.com/ipxe/ipxe/archive/"${sha_or_tag}".tar.gz
		mv "${archive_name}.tmp" "${archive_name}"
	else
		echo "already downloaded"
	fi
}

# extract_ipxe_repo will extract a tarball.
function extract_ipxe_repo() {
	local archive_name="$1"
	local archive_dir="$2"

	if [[ ! -d "${archive_dir}" ]]; then
		echo "extracting"
		mkdir -p "${archive_dir}.tmp"
		tar -xzf "${archive_name}" -C "${archive_dir}.tmp" --strip-components 1
		mv "${archive_dir}.tmp" "${archive_dir}"
	else
		echo "already extracted"
	fi
}

# patch_ipxe is needed to apply any patches needed to aid the build process.
# currently the only use case if for ipxe.iso building.
function patch_ipxe() {
	local archive_dir="$1"
	local patch_file="$2"

	echo "applying patch"
	patch --verbose -s -p1 -t -d "${archive_dir}" < "${patch_file}"
}

ipxe_sha_or_tag=$1
archive_name=ipxe-${ipxe_sha_or_tag}.tar.gz
download_ipxe_repo "${ipxe_sha_or_tag}" "${archive_name}"
extract_ipxe_repo "${archive_name}" "upstream-${ipxe_sha_or_tag}"
patch_ipxe "upstream-${ipxe_sha_or_tag}" "$2"
