OSFLAG:= $(shell go env GOHOSTOS)
IPXE_BUILD_SCRIPT:=binary/script/build_ipxe.sh
IPXE_NIX_SHELL:=binary/script/shell.nix

help: ## show this help message
		@grep -E '^[a-zA-Z_-]+.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

binary: binary/ipxe.efi binary/snp.efi binary/undionly.kpxe ## build all upstream ipxe binaries

# ipxe_sha_or_tag := v1.21.1 # could not get this tag to build ipxe.efi
# https://github.com/ipxe/ipxe/tree/2265a65191d76ce367913a61c97752ab88ab1a59
ipxe_sha_or_tag := $(shell cat binary/script/ipxe.commit)

# building iPXE on a Mac is troublesome and difficult to get working. For that reason, on a Mac, we build the iPXE binary using Docker.
ipxe_build_in_docker := $(shell if [ $(OSFLAG) = "darwin" ]; then echo true; else echo false; fi)

binary/ipxe.efi: ## build ipxe.efi
		${IPXE_BUILD_SCRIPT} bin-x86_64-efi/ipxe.efi "$(ipxe_sha_or_tag)" $(ipxe_build_in_docker) $@ "${IPXE_NIX_SHELL}"

binary/undionly.kpxe: ## build undionly.kpxe
		${IPXE_BUILD_SCRIPT} bin/undionly.kpxe "$(ipxe_sha_or_tag)" $(ipxe_build_in_docker) $@ "${IPXE_NIX_SHELL}"

binary/snp.efi: ## build snp.efi
		${IPXE_BUILD_SCRIPT} bin-arm64-efi/snp.efi "$(ipxe_sha_or_tag)" $(ipxe_build_in_docker) $@  "${IPXE_NIX_SHELL}" "CROSS_COMPILE=aarch64-unknown-linux-gnu-"

.PHONY: binary/clean
binary/clean: ## clean ipxe binaries, upstream ipxe source code directory, and ipxe source tarball
		rm -rf binary/ipxe.efi binary/snp.efi binary/undionly.kpxe
		rm -rf upstream-*
		rm -rf ipxe-*
