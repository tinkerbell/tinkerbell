# Treat the whole recipe as a one shell script/invocation instead of one-per-line
.ONESHELL:
# Use bash instead of plain sh
SHELL := bash
.SHELLFLAGS := -o pipefail -euc

# Detect if running on macOS and if Homebrew is installed; check for GNU Make and GNU coreutils/findutils/sed/tar
ifeq ($(shell uname), Darwin)
  MAKE_VERSION := $(shell $(MAKE) -v | awk '/GNU Make/ {print $$3}')
  ifeq ($(shell expr $(MAKE_VERSION) \< 4), 1)
    $(error "GNU Make 4.x is required (Current version: $(MAKE_VERSION)) Install it via Homebrew with 'brew install make' and use 'gmake' instead of 'make'.")
  endif
  ifeq ($(shell command -v brew 2>/dev/null), /usr/local/bin/brew)
    HOMEBREW_PREFIX := $(shell brew --prefix)
    PATH := $(HOMEBREW_PREFIX)/opt/coreutils/libexec/gnubin:$(HOMEBREW_PREFIX)/opt/gnu-sed/libexec/gnubin:$(HOMEBREW_PREFIX)/opt/gnu-tar/libexec/gnubin:$(HOMEBREW_PREFIX)/opt/findutils/libexec/gnubin:$(PATH)
  endif
endif

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null || true)
ifeq ($(GIT_TAG),)
	GIT_TAG := v0.0.0
endif
VERSION ?=
ifeq ($(VERSION),)
	VERSION := $(shell go run --buildvcs=true ./script/version/)
endif
CGO_ENABLED := 0
export CGO_ENABLED
# UPX compression reduces binary size by ~50-70% but only works on Linux
# - On Linux: COMPRESS defaults to true (automatic UPX compression)
# - On macOS/others: COMPRESS defaults to false (UPX binaries won't execute)
# - Override with: make COMPRESS=true cross-compile (CI/Linux environments)
ifeq ($(shell uname),Linux)
	COMPRESS := true
else
	COMPRESS := false
endif
UPX_BASEDIR := $(PWD)/build
LOCAL_ARCH := $(shell uname -m)
LOCAL_ARCH_ALT :=
ifeq ($(LOCAL_ARCH),x86_64)
	LOCAL_ARCH_ALT := amd64
else ifeq ($(LOCAL_ARCH),aarch64)
	LOCAL_ARCH_ALT := arm64
endif
GITHUB_REPOSITORY_OWNER ?= tinkerbell
HELM_REPO_NAME ?= ghcr.io/${GITHUB_REPOSITORY_OWNER}/charts

########### Tools variables ###########
# Tool versions
GOIMPORT_VER           := latest
CONTROLLER_GEN_VERSION := v0.18.0
BUF_VERSION            := v1.56.0
PROTOC_GEN_GO_GRPC_VER := v1.5.1  # must be in sync with the version in buf.gen.yaml
PROTOC_GEN_GO_VER      := v1.36.7 # must be in sync with the version in buf.gen.yaml
UPX_VER 			   := 4.2.4
GODEPGRAPH_VER 	       := v0.0.0-20240411160502-0f324ca7e282
GOLANGCI_LINT_VERSION  := v2.4.0

GORELEASER_VER := v2.12.2
GORELEASER_BIN := goreleaser

# Directories.
TOOLS_BIN_DIR := $(abspath bin)

# Tool binaries with versions
GOIMPORTS_BIN := goimports
GOIMPORTS := $(TOOLS_BIN_DIR)/$(GOIMPORTS_BIN)-$(GOIMPORT_VER)
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VERSION)
BUF_BIN := buf
BUF := $(TOOLS_BIN_DIR)/$(BUF_BIN)-$(BUF_VERSION)
PROTOC_GEN_GO_GRPC_BIN := protoc-gen-go-grpc
PROTOC_GEN_GO_GRPC := $(TOOLS_BIN_DIR)/$(PROTOC_GEN_GO_GRPC_BIN)-$(PROTOC_GEN_GO_GRPC_VER)
PROTOC_GEN_GO_BIN := protoc-gen-go
PROTOC_GEN_GO := $(TOOLS_BIN_DIR)/$(PROTOC_GEN_GO_BIN)-$(PROTOC_GEN_GO_VER)
UPX_BIN := upx
UPX := $(TOOLS_BIN_DIR)/$(UPX_BIN)-$(UPX_VER)-$(LOCAL_ARCH)
GODEPGRAPH_BIN := godepgraph
GODEPGRAPH := $(TOOLS_BIN_DIR)/$(GODEPGRAPH_BIN)-$(GODEPGRAPH_VER)
GORELEASER := $(TOOLS_BIN_DIR)/$(GORELEASER_BIN)-$(GORELEASER_VER)
#######################################
######### Container images variable #########
# `?=` will only set the variable if it is not already set by the environment
IMAGE_NAME       ?= tinkerbell/tinkerbell:latest
IMAGE_NAME_AGENT ?= tinkerbell/tink-agent:latest
#############################################

all: help

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120

.PHONY: build
build: generate $(GORELEASER) ## Build the Tinkerbell and Tink Agent binaries
	$(GORELEASER) build --clean

TEST_PKG ?=
TEST_PKGS :=
ifeq ($(TEST_PKG),)
	TEST_PKGS := ./...
else
	TEST_PKGS := ./$(TEST_PKG)/...
endif

.PHONY: test
test: ## Run go test
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ${TEST_PKGS}

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: $(GOIMPORTS) ## Run go fmt
	go fmt ./...
	$(GOIMPORTS) -w .

FILE_TO_NOT_INCLUDE_IN_COVERAGE := script/version/main.go|*.pb.go|zz_generated.deepcopy.go|facility_string.go|severity_string.go

.PHONY: coverage
coverage: test ## Show test coverage
## Filter out generated files
	cat coverage.txt | grep -v -E '$(FILE_TO_NOT_INCLUDE_IN_COVERAGE)' > coverage.out
	go tool cover -func=coverage.out
	mv coverage.out coverage.txt

.PHONY: ci-checks
ci-checks: .github/workflows/ci-checks.sh ## Run the ci-checks.sh script
	./.github/workflows/ci-checks.sh

.PHONY: ci
ci: ci-checks coverage lint vet ## Runs all the same validations and tests that run in CI

# Run go generate
generated_go_files := \
		smee/internal/syslog/facility_string.go \
		smee/internal/syslog/severity_string.go \

generate-go: $(generated_go_files) ## Run Go's generate command
smee/internal/syslog/facility_string.go: smee/internal/syslog/message.go
smee/internal/syslog/severity_string.go: smee/internal/syslog/message.go
	go generate -run=".*_string.go" ./...
	$(GOIMPORTS) -w .

.PHONY: generate-proto
generate-proto: $(BUF) $(PROTOC_GEN_GO_GRPC) $(PROTOC_GEN_GO) ## Generate code from proto files.
	$(BUF) generate
	$(MAKE) fmt

# Kubernetes CRD generation
.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate WebhookConfiguration and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd webhook paths="./..." output:crd:artifacts:config=crd/bases
	$(MAKE) fmt

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="script/boilerplate.go.txt" paths="./..."
	$(MAKE) fmt

.PHONY: dep-graph
dep-graph: $(GODEPGRAPH) ## Generate a dependency graph
	rm -rf out/dep-graph.txt out/dep-graph.png
	$(GODEPGRAPH) -s -novendor -onlyprefixes "github.com/tinkerbell/tinkerbell,./cmd/agent,./cmd/tinkerbell" ./cmd/agent ./cmd/tinkerbell > out/dep-graph.txt
	cat out/dep-graph.txt | dot -Txdot -o out/dep-graph.dot

######### Helm charts - start #########
helm-files := $(shell git ls-files helm/tinkerbell/ | grep -v helm/tinkerbell/docs)
helm-package: out/helm/tinkerbell-$(VERSION).tgz ## Helm chart for Tinkerbell
out/helm/tinkerbell-$(VERSION).tgz: $(helm-files)
	helm package -d out/helm/ helm/tinkerbell --version $(VERSION) --app-version $(VERSION)

.PHONY: helm-publish
helm-publish: out/helm/tinkerbell-$(VERSION).tgz ## Publish the Helm chart
	helm push out/helm/tinkerbell-$(VERSION).tgz oci://$(HELM_REPO_NAME)

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint helm/tinkerbell --set "trustedProxies={127.0.0.1/24}" --set "publicIP=1.1.1.1" --set "artifactsFileServer=http://2.2.2.2"

.PHONY: helm-template
helm-template: ## Helm template for Tinkerbell
	helm template test helm/tinkerbell --set "trustedProxies={127.0.0.1/24}" --set "publicIP=1.1.1.1" --set "artifactsFileServer=http://2.2.2.2" 2>&1 >/dev/null

######### Helm charts - end   #########

######### Build container images - start #########
.PHONY: build-image
build-image: $(GORELEASER) ## Build the container images
	$(GORELEASER) release --clean --skip=sign --verbose

.PHONY: build-image-push
build-image-push: $(GORELEASER) ## Build and push the container images
	GORELEASER_CURRENT_TAG=$(shell git rev-parse HEAD) $(GORELEASER) release --clean --skip=validate --skip=sign ${GORELEASER_EXTRA_FLAGS}

######### Build container images - end   #########

.PHONY: clean
clean: ## Remove all generated binaries
	rm -rf dist out

.PHONY: clean-tools
clean-tools: ## Remove all tools
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-all
clean-all: clean clean-tools ## Remove all binaries and tools

############## Tools ##############
$(GOIMPORTS):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install golang.org/x/tools/cmd/goimports@$(GOIMPORT_VER)
	@mv $(TOOLS_BIN_DIR)/goimports $(GOIMPORTS)

$(CONTROLLER_GEN):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	@mv $(TOOLS_BIN_DIR)/controller-gen $(CONTROLLER_GEN)

$(PROTOC_GEN_GO_GRPC):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	@mv $(TOOLS_BIN_DIR)/protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC)

$(PROTOC_GEN_GO):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VER)
	@mv $(TOOLS_BIN_DIR)/protoc-gen-go $(PROTOC_GEN_GO)

$(BUF):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@mv $(TOOLS_BIN_DIR)/buf $(BUF)

$(UPX):
	mkdir -p $(TOOLS_BIN_DIR)
ifeq ($(shell uname),Darwin)
	@echo "Downloading UPX for macOS (using amd64 with Rosetta 2 compatibility)..."
	(cd $(TOOLS_BIN_DIR); curl -sSfLO https://github.com/upx/upx/releases/download/v$(UPX_VER)/upx-$(UPX_VER)-amd64_linux.tar.xz)
	(cd $(TOOLS_BIN_DIR); tar -xf upx-$(UPX_VER)-amd64_linux.tar.xz)
	@chmod +x $(TOOLS_BIN_DIR)/upx-$(UPX_VER)-amd64_linux/upx
	@mv $(TOOLS_BIN_DIR)/upx-$(UPX_VER)-amd64_linux/upx $(UPX)
	@rm -rf $(TOOLS_BIN_DIR)/upx-$(UPX_VER)-amd64_linux*
else
	(cd $(TOOLS_BIN_DIR); curl -sSfLO https://github.com/upx/upx/releases/download/v$(UPX_VER)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	(cd $(TOOLS_BIN_DIR); tar -xf upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	@mv $(TOOLS_BIN_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux/upx $(UPX)
	@rm -rf $(TOOLS_BIN_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux*
endif

$(GODEPGRAPH):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/kisielk/godepgraph@$(GODEPGRAPH_VER)
	@mv $(TOOLS_BIN_DIR)/godepgraph $(GODEPGRAPH)

$(GORELEASER):
	mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VER)
	@mv $(TOOLS_BIN_DIR)/goreleaser $(GORELEASER)

.PHONY: tools
tools: $(GOIMPORTS) $(CONTROLLER_GEN) $(PROTOC_GEN_GO_GRPC) $(PROTOC_GEN_GO) $(BUF) $(UPX) $(GODEPGRAPH) $(GORELEASER) ## Install all tools

############## Linting ##############
.PHONY: lint
lint: _lint  ## Run linting

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

GOLANGCI_LINT_CONFIG := $(LINT_ROOT)/.golangci.yml
GOLANGCI_LINT_BIN := $(LINT_ROOT)/out/linters/golangci-lint-$(GOLANGCI_LINT_VERSION)-$(LINT_ARCH)
$(GOLANGCI_LINT_BIN):
	mkdir -p $(LINT_ROOT)/out/linters
	rm -rf $(LINT_ROOT)/out/linters/golangci-lint-*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LINT_ROOT)/out/linters $(GOLANGCI_LINT_VERSION)
	mv $(LINT_ROOT)/out/linters/golangci-lint $@

LINTERS += golangci-lint-lint
golangci-lint-lint: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -not -path "./out/*" -execdir sh -c '"$(GOLANGCI_LINT_BIN)" run --timeout 10m -c "$(GOLANGCI_LINT_CONFIG)"' '{}' '+'

FIXERS += golangci-lint-fix
golangci-lint-fix: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -not -path "./out/*" -execdir "$(GOLANGCI_LINT_BIN)" run -c "$(GOLANGCI_LINT_CONFIG)" --fix \;

.PHONY: _lint $(LINTERS)
_lint: $(LINTERS)

.PHONY: fix $(FIXERS)
fix: $(FIXERS)
