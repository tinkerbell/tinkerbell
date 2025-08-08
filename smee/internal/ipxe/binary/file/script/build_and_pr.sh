#!/bin/bash

set -uxo pipefail

# tracked_files defines the files that will cause the iPXE binaries to be rebuilt.
tracked_files=(
    "./script/build_ipxe.sh"
    "./script/build_and_pr.sh"
    "./script/ipxe-customizations/console.h"
    "./script/ipxe-customizations/isa.h"
    "./script/ipxe-customizations/colour.h"
    "./script/ipxe-customizations/crypto.h"
    "./script/ipxe-customizations/general.efi.h"
    "./script/ipxe-customizations/general.undionly.h"
    "./script/ipxe-customizations/common.h"
    "./script/ipxe-customizations/nap.h"
    "./script/embed.ipxe"
    "./script/ipxe.commit"
    "./ipxe.efi"
    "./snp-arm64.efi"
    "./snp-x86_64.efi"
    "./undionly.kpxe"
    "./ipxe.iso"
    "./ipxe-efi.img"
)

# binaries defines the files that will be built if any tracked_files changes are detected.
binaries=(
    "script/sha512sum.txt"
    "snp-arm64.efi"
    "snp-x86_64.efi"
    "ipxe.efi"
    "undionly.kpxe"
    "ipxe.iso"
    "ipxe-efi.img"
)

# check for the GITHUB_TOKEN environment variable
function check_github_token() {
  if [ -z "${GITHUB_TOKEN}" ]; then
    echo "GITHUB_TOKEN is not set"
    exit 1
  fi
  echo "Checking for 'gh' CLI"
  if ! command -v gh &> /dev/null; then
    echo "'gh' CLI is not installed. Please install it to use this script."
    exit 3
  fi
}

# check for changes to iPXE files
function changes_detected() {
    local file="${1:-sha512sum.txt}"

    if create_checksums /dev/stdout | diff -U 1 "${file}" -; then
        echo "No changes detected"
        exit 0
    fi
    echo "Changes detected"
}

# remove old iPXE files
function clean_iPXE() {
    # remove existing iPXE binaries
    echo "Removing existing iPXE binaries"
    if ! make clean; then
        echo "Failed to remove iPXE binaries" 1>&2
        exit 1
    fi
}

# build iPXE binaries
function build_iPXE() {
    # build iPXE
    echo "Building iPXE"
    if ! (nix-shell "script/shell.nix" --run 'make binary'); then
        echo "Failed to build iPXE" 1>&2
        exit 1
    fi
}

# update checksums file
function create_checksums() {
    local location="${1:-sha512sum.txt}"

    if ! sha512sum "${tracked_files[@]}" > "${location}"; then
        echo "Failed to create checksums file" 1>&2
        exit 1
    fi
}

# push a new branch to GitHub
function push_new_branch_to_github() {
    local branch="${1}"
    local repository="${2:-tinkerbell/tinkerbell}"
    local git_actor="${3:-github-actions[bot]}"
    local token="${4:-${GITHUB_TOKEN}}"

    # only push if there are no changes from main
    if ! git diff --quiet main; then
        echo "Changes detected from main, not pushing"
        exit 1
    fi

    # push changes
    echo "Pushing changes"
    # increase the postBuffer size to allow for large commits. ipxe.iso is 2mb in size.
    git config --global http.postBuffer 157286400
    if ! git push https://"${git_actor}":"${token}"@github.com/"${repository}".git HEAD:"${branch}"; then
        echo "Failed to push branch to GitHub" 1>&2
        exit 1
    fi
}

# create a new branch
function create_branch() {
    local branch="${1:-update_iPXE_$(date +"%Y_%m_%d_%H_%M_%S")}"

    # create a new branch
    if ! git checkout -b "${branch}"; then
        echo "Failed to create branch ${branch}" 1>&2
        exit 1
    fi
    # push the new branch to GitHub
    if ! push_new_branch_to_github "${branch}"; then
        echo "Failed to push branch ${branch} to GitHub" 1>&2
        exit 1
    fi
    echo "Branch ${branch} created and pushed to GitHub"
}

# create Github Pull Request
function create_pull_request() {
    local branch="$1"
    local base="${2:-main}"
    local title="${3:-Update iPXE binaries}"
    local body="${4:-updated iPXE binaries}"

    # create pull request
    echo "Creating pull request"
    if ! gh pr create --base "${base}" --body "${body}" --title "${title}" --head "${branch}"; then
        echo "Failed to create pull request" 1>&2
        exit 1
    fi
}

function main() {
    local task="$1"
    local branch="$2"
    local sha_file="$3"

    if [ "${task}" == "build" ]; then
        # Build iPXE binaries
        check_github_token
        changes_detected "${sha_file}"
        echo "Building iPXE binaries"
        branch="update_iPXE_$(date +"%Y_%m_%d_%H_%M_%S")"
        create_branch "${branch}"
        clean_iPXE
        build_iPXE
        create_checksums "${sha_file}"
        if [ -n "${GITHUB_OUTPUT:-}" ]; then
            echo "new_binaries_built=true" >> "$GITHUB_OUTPUT"
            echo "new_branch_name=${branch}" >> "$GITHUB_OUTPUT"
        else
            echo "not running in GitHub Actions"
        fi
        return 0
    fi

    if [ "${task}" == "pr" ]; then
        echo "Creating pull request"
        # Create pull request
        check_github_token
        create_pull_request "${branch}" "main" "Update iPXE binaries" "Automated iPXE binaries update."
        return 0
    fi

    echo "Unknown task: ${task}" 1>&2
    exit 1
}

main "${1:-build}" "${2:-}" "${3:-./script/sha512sum.txt}"
