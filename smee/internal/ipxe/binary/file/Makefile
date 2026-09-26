IPXE_BUILD_SCRIPT := script/build_ipxe.sh
IPXE_FETCH_SCRIPT := script/fetch_and_extract_ipxe.sh
IPXE_NIX_SHELL := script/shell.nix
IPXE_BUILD_PATCHES := $(sort $(wildcard script/*.patch))
BINARIES := ipxe.efi snp-arm64.efi snp-x86_64.efi undionly.kpxe ipxe.iso ipxe-efi.img

help: ## show this help message
	@grep -E '^[a-zA-Z_-]+.*:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

# building iPXE on a Mac is troublesome and difficult to get working. It is recommended to build in Docker.
in-docker: ## Run nix Docker container
	docker run --rm -v $(shell pwd):/code -w /code -it nixos/nix nix-shell script/shell.nix

.PHONY: binary
binary: $(BINARIES) ## build all upstream ipxe binaries

# ipxe_sha_or_tag := v1.21.1 # could not get this tag to build ipxe.efi
# https://github.com/ipxe/ipxe/tree/2265a65191d76ce367913a61c97752ab88ab1a59
ipxe_sha_or_tag := $(shell cat script/ipxe.commit)
# Key the marker on the patch set. Make cannot see a prerequisite that no longer
# exists, so without this a deleted patch would leave the tree patched.
ipxe_patch_id := $(shell cat /dev/null $(IPXE_BUILD_PATCHES) | sha256sum | cut -c1-12)
ipxe_stamp := upstream-$(ipxe_sha_or_tag)/.extracted-$(ipxe_patch_id)

.PHONY: extract-ipxe
extract-ipxe: $(ipxe_stamp) ## Fetch and extract ipxe source
# Patches are applied to the extracted tree, so it has to be thrown away and
# re-extracted; re-running the fetch script over it would stack them.
$(ipxe_stamp): script/ipxe.commit $(IPXE_BUILD_PATCHES)
	rm -rf "upstream-$(ipxe_sha_or_tag)"
	${IPXE_FETCH_SCRIPT} "$(ipxe_sha_or_tag)" ${IPXE_BUILD_PATCHES}
	touch "$@"

ipxe.efi: $(ipxe_stamp) ## build ipxe.efi
	+${IPXE_BUILD_SCRIPT} bin-x86_64-efi/ipxe.efi "$(ipxe_sha_or_tag)" $@

undionly.kpxe: $(ipxe_stamp) ## build undionly.kpxe
	+${IPXE_BUILD_SCRIPT} bin/undionly.kpxe "$(ipxe_sha_or_tag)" $@

snp-arm64.efi: $(ipxe_stamp) ## build snp-arm64.efi
	+${IPXE_BUILD_SCRIPT} bin-arm64-efi/snp.efi "$(ipxe_sha_or_tag)" $@ "CROSS_COMPILE=aarch64-unknown-linux-gnu-"

ipxe.iso: $(ipxe_stamp) ## build ipxe.iso
	+${IPXE_BUILD_SCRIPT} bin-x86_64-efi/ipxe.iso "$(ipxe_sha_or_tag)" $@

snp-x86_64.efi: $(ipxe_stamp) ## build snp-x86_64.efi
	+${IPXE_BUILD_SCRIPT} bin-x86_64-efi/snp.efi "$(ipxe_sha_or_tag)" $@

.DELETE_ON_ERROR: ipxe-efi.img
ipxe-efi.img: ipxe.efi ## build ipxe-efi.img
	qemu-img create -f raw $@ 1440K
	sgdisk --clear --set-alignment=34 --new 1:34:-0 --typecode=1:EF00 --change-name=1:"IPXE" $@
	mkfs.vfat -F 12 -n "IPXE" --offset 34 $@ 1400
	mmd -i $@@@17K ::/EFI
	mmd -i $@@@17K ::/EFI/BOOT
	mcopy -i $@@@17K $< ::/EFI/BOOT/BOOTX64.efi

.PHONY: clean
clean: ## clean ipxe binaries, upstream ipxe source code directory, and ipxe source tarball
	rm -rf $(BINARIES)
	rm -rf upstream-*
	rm -rf ipxe-*
