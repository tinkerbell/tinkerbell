#!/usr/bin/env bash
# This script handles all the steps needed to
# download and compile ipxe from source.

set -eux

# download_ipxe_repo will download a source archive from github.
function download_ipxe_repo() {
    local sha_or_tag="$1"
    if [ ! -f "ipxe-${sha_or_tag}.tar.gz" ]; then
        echo "downloading"
        curl -fLo ipxe-"${sha_or_tag}".tar.gz https://github.com/ipxe/ipxe/archive/"${sha_or_tag}".tar.gz
    else
        echo "already downloaded"
    fi
    
}

# extract_ipxe_repo will extract a tarball.
function extract_ipxe_repo() {
    local archive_name="$1"
    local archive_dir="$2"

    if [ ! -d "$archive_dir" ]; then
        echo "extracting"
        mkdir -p "${archive_dir}"
        tar -xzf "${archive_name}" -C "${archive_dir}" --strip-components 1
    else
        echo "already extracted"
    fi
}

# build_ipxe will run the make target in the upstream ipxe source
# that will build an ipxe binary.
function build_ipxe() {
    local ipxe_dir="$1"
    local ipxe_bin="$2"
    local run_in_docker="$3"
    local env_opts="$4"
    local embed_path="$5"
    local nix_shell="$6"

    if [ "${run_in_docker}" = true ]; then
        if [ ! -f "${ipxe_dir}/src/${ipxe_bin}" ]; then
            echo "running in docker"
            docker run -it --rm -v "${PWD}":/code -w /code nixos/nix nix-shell "${nix_shell}" --run "${env_opts} make -C ${ipxe_dir}/src EMBED=${embed_path} ${ipxe_bin}"
        fi
    else
        echo "running locally"
        nix-shell "${nix_shell}" --run "${env_opts} make -C ${ipxe_dir}/src EMBED=${embed_path} ${ipxe_bin}"
    fi
}

# mv_embed_into_build will move an ipxe script into a location available
# to the ipxe build so that it can be embedded into an ipxe binary.
function mv_embed_into_build() {
    local embed_path="$1"
    local ipxe_dir="$2"

    cp -a "${embed_path}" "${ipxe_dir}"/src/embed.ipxe
}

# make_local_empty will delete any custom ipxe header files,
# putting the ipxe src back to a known good/clean state.
function make_local_empty() {
    local ipxe_dir="$1" 

    rm -rf "${ipxe_dir}"/src/config/local/*
}

# copy_common_files will copy common custom header files into the ipxe src path.
function copy_common_files() {
    local ipxe_dir="$1" 
    cp -a binary/script/ipxe-customizations/colour.h "${ipxe_dir}"/src/config/local/
    cp -a binary/script/ipxe-customizations/common.h "${ipxe_dir}"/src/config/local/
    cp -a binary/script/ipxe-customizations/console.h "${ipxe_dir}"/src/config/local/
}

# copy_custom_files will copy in any custom header files based on a requested ipxe binary.
function copy_custom_files() {
    local ipxe_dir="$1"
    local ipxe_bin="$2"

    case "${ipxe_bin}" in
    bin/undionly.kpxe)
    	cp binary/script/ipxe-customizations/general.undionly.h "${ipxe_dir}"/src/config/local/general.h
    	;;
    bin/ipxe.lkrn)
    	cp binary/script/ipxe-customizations/general.undionly.h "${ipxe_dir}"/src/config/local/general.h
    	;;
    bin-x86_64-efi/ipxe.efi)
    	cp binary/script/ipxe-customizations/general.efi.h "${ipxe_dir}"/src/config/local/general.h
        cp binary/script/ipxe-customizations/isa.h "${ipxe_dir}"/src/config/local/isa.h
    	;;
    bin-arm64-efi/snp.efi)
    	cp binary/script/ipxe-customizations/general.efi.h "${ipxe_dir}"/src/config/local/general.h
    	;;
    *) echo "unknown binary: ${ipxe_bin}" >&2 && exit 1 ;;
    esac
}

# customize_aarch_build will modify a make file for arm64 builds.
# see http://lists.ipxe.org/pipermail/ipxe-devel/2018-August/006254.html .
function customize_aarch_build() {
    local ipxe_dir="$1"
    # http://lists.ipxe.org/pipermail/ipxe-devel/2018-August/006254.html
    sed -i.bak '/^WORKAROUND_CFLAGS/ s|^|#|' "${ipxe_dir}"/src/arch/arm64/Makefile
}

# customize orchestrates the process for adding custom headers to an ipxe compile.
function customize() {
    local ipxe_dir="$1"
    local ipxe_bin="$2"

    make_local_empty "${ipxe_dir}"
    copy_common_files "${ipxe_dir}"
    copy_custom_files "${ipxe_dir}" "${ipxe_bin}"
    customize_aarch_build "${ipxe_dir}"
}

function hasType() {
    if [ -z "$(type type)" ]; then
        echo "type command not found"
        return 1
    fi
}

function hasDocker() {
    if [ -z "$(type docker)" ]; then
        echo "docker command not found"
        return 1
    fi
}

function hasNixShell() {
    if [ -z "$(type nix-shell)" ]; then
        echo "nix-shell command not found"
        return 1
    fi
}

function hasUname() {
    if [ -z "$(type uname)" ]; then
        echo "uname command not found"
        return 1
    fi
}

# main function orchestrating a full ipxe compile.
function main() {
    local bin_path
    bin_path=$(echo "${1}" | xargs)
    local ipxe_sha_or_tag
    ipxe_sha_or_tag=$(echo "${2}" | xargs)
    local ipxe_build_in_docker
    ipxe_build_in_docker=$(echo "${3}" | xargs)
    local final_path
    final_path=$(echo "${4}" | xargs)
    local nix_shell
    nix_shell=$(echo "${5}" | xargs)
    local env_opts
    env_opts=$(echo "${6}" | xargs)
    local embed_path
    embed_path=$(echo "${7}" | xargs)

    # check for prerequisites
    hasType
    hasNixShell
    hasUname
    local OS_TEST
    OS_TEST=$(uname | tr '[:upper:]' '[:lower:]')
    if [[ "${OS_TEST}" != *"linux"* ]]; then
        hasDocker
    fi

    download_ipxe_repo "${ipxe_sha_or_tag}"
    extract_ipxe_repo "ipxe-${ipxe_sha_or_tag}.tar.gz" "upstream-${ipxe_sha_or_tag}"
    mv_embed_into_build "${embed_path}" "upstream-${ipxe_sha_or_tag}"
    customize "upstream-${ipxe_sha_or_tag}" "${bin_path}"
    build_ipxe "upstream-${ipxe_sha_or_tag}" "${bin_path}" "${ipxe_build_in_docker}" "${env_opts}" "embed.ipxe" "${nix_shell}"
    cp -a "upstream-${ipxe_sha_or_tag}/src/${bin_path}" "${final_path}"
}

main "$1" "$2" "$3" "$4" "$5" "${6:-''}" "${7:-binary/script/embed.ipxe}"
