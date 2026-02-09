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
  ifneq ($(shell command -v brew 2>/dev/null), "")
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
GITHUB_REPOSITORY_OWNER ?= $(shell git remote get-url $(shell git remote | head -n1 2>/dev/null) 2>/dev/null | sed -E 's|.*github.com[:/]([^/]+)/.*|\1|' || echo "tinkerbell")
HELM_REPO_NAME ?= ghcr.io/${GITHUB_REPOSITORY_OWNER}/charts

########### Tools variables ###########
# Tool versions
CONTROLLER_GEN_VER	:= v0.20.0
BUF_VER							:= v1.56.0
UPX_VER							:= 4.2.4
GODEPGRAPH_VER			:= v0.0.0-20240411160502-0f324ca7e282
GOLANGCI_LINT_VER		:= v2.8.0
GORELEASER_VER			:= v2.12.2

# Directories.
TOOLS_BIN_DIR := $(abspath bin)

# Tool binaries with versions
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER)
BUF_BIN := buf
BUF := $(TOOLS_BIN_DIR)/$(BUF_BIN)-$(BUF_VER)
UPX_BIN := upx
UPX := $(TOOLS_BIN_DIR)/$(UPX_BIN)-$(UPX_VER)-$(LOCAL_ARCH)
GODEPGRAPH_BIN := godepgraph
GODEPGRAPH := $(TOOLS_BIN_DIR)/$(GODEPGRAPH_BIN)-$(GODEPGRAPH_VER)
GORELEASER_BIN := goreleaser
GORELEASER := $(TOOLS_BIN_DIR)/$(GORELEASER_BIN)-$(GORELEASER_VER)
GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN)-$(GOLANGCI_LINT_VER)-$(LOCAL_ARCH)

#######################################
######### Container images variable #########
# `?=` will only set the variable if it is not already set by the environment
IMAGE_NAME	?= $(GITHUB_REPOSITORY_OWNER)/tinkerbell
REGISTRY		?= ghcr.io
GORELEASER_EXTRA_ENV ?= IMAGE_NAME=$(IMAGE_NAME) REGISTRY=$(REGISTRY)
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
fmt: $(GOLANGCI_LINT) ## Run go fmt
	$(GOLANGCI_LINT) fmt ./...

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

.PHONY: generate-proto
generate-proto: $(BUF) $(PROTOC_GEN_GO_GRPC) $(PROTOC_GEN_GO) ## Generate code from proto files.
	$(BUF) generate
	$(GOLANGCI_LINT) fmt --enable goimports --config <(printf 'version: "2"\nformatters:\n  exclusions:\n    generated: disable') ./...

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
build-image: | $(GORELEASER) ## Build the container images
	$(GORELEASER) release --clean --skip=sign --verbose

.PHONY: build-image-push
build-image-push: | $(GORELEASER) ## Build and push the container images
	@$(GORELEASER_EXTRA_ENV) $(GORELEASER) release --clean ${GORELEASER_EXTRA_FLAGS}

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
$(TOOLS_BIN_DIR):
	mkdir -p $@

$(CONTROLLER_GEN): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VER)
	@mv $(TOOLS_BIN_DIR)/controller-gen $(CONTROLLER_GEN)

$(PROTOC_GEN_GO_GRPC): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	@mv $(TOOLS_BIN_DIR)/protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC)

$(PROTOC_GEN_GO): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VER)
	@mv $(TOOLS_BIN_DIR)/protoc-gen-go $(PROTOC_GEN_GO)

$(BUF): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VER)
	@mv $(TOOLS_BIN_DIR)/buf $(BUF)

$(UPX): | $(TOOLS_BIN_DIR)
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

$(GODEPGRAPH): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/kisielk/godepgraph@$(GODEPGRAPH_VER)
	@mv $(TOOLS_BIN_DIR)/$(GODEPGRAPH_BIN) $(GODEPGRAPH)

$(GORELEASER): | $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VER)
	@mv $(TOOLS_BIN_DIR)/$(GORELEASER_BIN) $(GORELEASER)

$(GOLANGCI_LINT): | $(TOOLS_BIN_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) $(GOLANGCI_LINT_VER)
	@mv $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN) $(GOLANGCI_LINT)

.PHONY: tools
tools: $(CONTROLLER_GEN) $(PROTOC_GEN_GO_GRPC) $(PROTOC_GEN_GO) $(BUF) $(UPX) $(GODEPGRAPH) $(GORELEASER) $(GOLANGCI_LINT) ## Install all tools

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
LINTERS += golangci-lint-lint
golangci-lint-lint: $(GOLANGCI_LINT)
	find . -name go.mod -not -path "./out/*" -execdir sh -c '"$(GOLANGCI_LINT)" run --timeout 10m -c "$(GOLANGCI_LINT_CONFIG)"' '{}' '+'

FIXERS += golangci-lint-fix
golangci-lint-fix: $(GOLANGCI_LINT)
	find . -name go.mod -not -path "./out/*" -execdir "$(GOLANGCI_LINT)" run -c "$(GOLANGCI_LINT_CONFIG)" --fix \;

.PHONY: _lint $(LINTERS)
_lint: $(LINTERS)

.PHONY: fix $(FIXERS)
fix: $(FIXERS)
