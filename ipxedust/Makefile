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

.PHONY: test
test: ## run unit tests
	go test -v -covermode=count ./...

.PHONY: cover
cover: ## Run unit tests with coverage report
	go test -coverprofile=coverage.out ./... || true
	go tool cover -func=coverage.out

# BEGIN: lint-install /Users/jacobweinstock/repos/tinkerbell/boots-ipxe
# http://github.com/tinkerbell/lint-install

.PHONY: lint
lint: _lint

LINT_ARCH := $(shell uname -m)
LINT_OS := $(shell uname)
LINT_OS_LOWER := $(shell echo $(LINT_OS) | tr '[:upper:]' '[:lower:]')
LINT_ROOT := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# shellcheck and hadolint lack arm64 native binaries: rely on x86-64 emulation
ifeq ($(LINT_OS),Darwin)
	ifeq ($(LINT_ARCH),arm64)
		LINT_ARCH=x86_64
	endif
endif

LINTERS :=
FIXERS :=

SHELLCHECK_VERSION ?= v0.8.0
SHELLCHECK_BIN := out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)
$(SHELLCHECK_BIN):
	mkdir -p out/linters
	rm -rf out/linters/shellcheck-*
	curl -sSfL https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).$(LINT_OS_LOWER).$(LINT_ARCH).tar.xz | tar -C out/linters -xJf -
	mv out/linters/shellcheck-$(SHELLCHECK_VERSION)/shellcheck $@
	rm -rf out/linters/shellcheck-$(SHELLCHECK_VERSION)/shellcheck

LINTERS += shellcheck-lint
shellcheck-lint: $(SHELLCHECK_BIN)
	$(SHELLCHECK_BIN) $(shell find . -name "*.sh")

FIXERS += shellcheck-fix
shellcheck-fix: $(SHELLCHECK_BIN)
	$(SHELLCHECK_BIN) $(shell find . -name "*.sh") -f diff | { read -t 1 line || exit 0; { echo "$$line" && cat; } | git apply -p2; }

GOLANGCI_LINT_CONFIG := $(LINT_ROOT)/.golangci.yml
GOLANGCI_LINT_VERSION ?= v1.43.0
GOLANGCI_LINT_BIN := out/linters/golangci-lint-$(GOLANGCI_LINT_VERSION)-$(LINT_ARCH)
$(GOLANGCI_LINT_BIN):
	mkdir -p out/linters
	rm -rf out/linters/golangci-lint-*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b out/linters $(GOLANGCI_LINT_VERSION)
	mv out/linters/golangci-lint $@

LINTERS += golangci-lint-lint
golangci-lint-lint: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -execdir "$(GOLANGCI_LINT_BIN)" run -c "$(GOLINT_CONFIG)" \;

FIXERS += golangci-lint-fix
golangci-lint-fix: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -execdir "$(GOLANGCI_LINT_BIN)" run -c "$(GOLINT_CONFIG)" --fix \;

YAMLLINT_VERSION ?= 1.26.3
YAMLLINT_ROOT := out/linters/yamllint-$(YAMLLINT_VERSION)
YAMLLINT_BIN := $(YAMLLINT_ROOT)/dist/bin/yamllint
$(YAMLLINT_BIN):
	mkdir -p out/linters
	rm -rf out/linters/yamllint-*
	curl -sSfL https://github.com/adrienverge/yamllint/archive/refs/tags/v$(YAMLLINT_VERSION).tar.gz | tar -C out/linters -zxf -
	cd $(YAMLLINT_ROOT) && pip3 install --target dist .

LINTERS += yamllint-lint
yamllint-lint: $(YAMLLINT_BIN)
	PYTHONPATH=$(YAMLLINT_ROOT)/dist $(YAMLLINT_ROOT)/dist/bin/yamllint .

.PHONY: _lint $(LINTERS)
_lint: $(LINTERS)

.PHONY: fix $(FIXERS)
fix: $(FIXERS)

# END: lint-install /Users/jacobweinstock/repos/tinkerbell/boots-ipxe
