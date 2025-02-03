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
    "./snp.efi"
    "./undionly.kpxe"
    "./ipxe.iso"
    "./ipxe-efi.img"
)

# binaries defines the files that will be built if any tracked_files changes are detected.
binaries=(
    "script/sha512sum.txt"
    "snp.efi"
    "ipxe.efi"
    "undionly.kpxe"
    "ipxe.iso"
    "ipxe-efi.img"
)

git_email="github-actions[bot]@users.noreply.github.com"
git_name="github-actions[bot]"
repo="tinkerbell/tinkerbell"

# check for the GITHUB_TOKEN environment variable
function check_github_token() {
  if [ -z "${GITHUB_TOKEN}" ]; then
    echo "GITHUB_TOKEN is not set"
    exit 1
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

# configure git client
function configure_git() {
    local email="${1:-github-actions[bot]@users.noreply.github.com}"
    local name="${2:-github-actions[bot]}"

    if ! git config --local user.email "${email}"; then
        echo "Failed to configure git user.email" 1>&2
        exit 1
    fi
    if ! git config --local user.name "${name}"; then
        echo "Failed to configure git user.name" 1>&2
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
    if ! push_changes "${branch}"; then
        echo "Failed to push branch ${branch}" 1>&2
        exit 1
    fi
}

# shellcheck disable=SC2086
# commit changes to git
function commit_changes() {
    local files="${1:-script/sha512sum.txt snp.efi ipxe.efi undionly.kpxe ipxe.iso}"
    local message="${2:-Updated iPXE}"

    # commit changes
    echo "Committing changes"
    if ! git add ${files}; then
        echo "Failed to add changes" 1>&2
        exit 1
    fi
    if ! git commit -sm "${message}"; then
        echo "Failed to commit changes" 1>&2
        exit 1
    fi
}

# push changes to origin
function push_changes() {
    local branch="${1}"
    local repository="${2:-tinkerbell/tinkerbell}"
    local git_actor="${3:-github-actions[bot]}"
    local token="${4:-${GITHUB_TOKEN}}"

    # push changes
    echo "Pushing changes"
    # increase the postBuffer size to allow for large commits. ipxe.iso is 2mb in size.
    git config --global http.postBuffer 157286400
    if ! git push https://"${git_actor}":"${token}"@github.com/"${repository}".git HEAD:"${branch}"; then
        echo "Failed to push changes" 1>&2
        exit 1
    fi
}

# create Github Pull Request
function create_pull_request() {
    local branch="$1"
    local base="${2:-main}"
    local title="${3:-Update iPXE binaries}"
    local body="${4:-updated iPXE binaries}"

    # create pull request
    echo "Creating pull request"
    if ! script/gh pr create --base "${base}" --body "${body}" --title "${title}" --head "${branch}"; then
        echo "Failed to create pull request" 1>&2
        exit 1
    fi
}

# clean_up undoes any changes made by the script
function clean_up() {
    if ! git config --local --unset user.email; then
        echo "Failed to unset git user.email" 1>&2
        exit 1
    fi
    if ! git config --local --unset user.name; then
        echo "Failed to unset git user.name" 1>&2
        exit 1
    fi
}

function main() {
    local sha_file="$1"

    check_github_token
    changes_detected "${sha_file}"
    branch="update_iPXE_$(date +"%Y_%m_%d_%H_%M_%S")"
    create_branch "${branch}"
    clean_iPXE
    build_iPXE
    create_checksums "${sha_file}"
    configure_git "${git_email}" "${git_name}"
    # shellcheck disable=SC2068,SC2145
    commit_changes "$(printf "%s " "${binaries[@]}"|xargs)" "Updated iPXE binaries"
    push_changes "${branch}" "${repo}" "${git_name}" "${GITHUB_TOKEN}"
    create_pull_request "${branch}" "main" "Update iPXE binaries" "Automated iPXE binaries update."
    clean_up
}

main "${1:-./script/sha512sum.txt}"
