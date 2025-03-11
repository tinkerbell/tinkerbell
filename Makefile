# Treat the whole recipe as a one shell script/invocation instead of one-per-line
.ONESHELL:
# Use bash instead of plain sh
SHELL := bash
.SHELLFLAGS := -o pipefail -euc

GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null || true)
ifeq ($(GIT_TAG),)
	GIT_TAG := v0.0.0
endif
VERSION ?=
ifeq ($(VERSION),)
	VERSION := $(GIT_TAG)-$(GIT_COMMIT)
endif
CGO_ENABLED := 0
export CGO_ENABLED
COMPRESS := false
UPX_BASEDIR := $(PWD)/build
LOCAL_ARCH := $(shell uname -m)
LOCAL_ARCH_ALT :=
ifeq ($(LOCAL_ARCH),x86_64)
	LOCAL_ARCH_ALT := amd64
else ifeq ($(LOCAL_ARCH),aarch64)
	LOCAL_ARCH_ALT := arm64
endif
HELM_REPO_NAME ?= ghcr.io/tinkerbell/charts

########### Tools variables ###########
# Tool versions
GOIMPORT_VER           := latest
CONTROLLER_GEN_VERSION := v0.17.1
BUF_VERSION            := v1.50.0
PROTOC_GEN_GO_GRPC_VER := v1.5.1
PROTOC_GEN_GO_VER      := v1.36.5
UPX_VER 			   := 4.2.4
GODEPGRAPH_VER 	       := v0.0.0-20240411160502-0f324ca7e282

# Tool fully qualified paths (FQP)
TOOLS_DIR := $(PWD)/out/tools
GOIMPORTS_FQP := $(TOOLS_DIR)/goimports-$(GOIMPORT_VER)
CONTROLLER_GEN_FQP := $(TOOLS_DIR)/controller-gen-$(CONTROLLER_GEN_VERSION)
BUF_FQP := $(TOOLS_DIR)/buf-$(BUF_VERSION)
PROTOC_GEN_GO_GRPC_FQP := $(TOOLS_DIR)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VER)
PROTOC_GEN_GO_FQP := $(TOOLS_DIR)/protoc-gen-go-$(PROTOC_GEN_GO_VER)
UPX_FQP := $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH)
GODEPGRAPH_FQP := $(TOOLS_DIR)/godepgraph-$(GODEPGRAPH_VER)
#######################################
######### Container images variable #########
# `?=` will only set the variable if it is not already set by the environment
IMAGE_NAME       ?= tinkerbell/tinkerbell:latest
IMAGE_NAME_AGENT ?= tinkerbell/tink-agent:latest
#############################################

all: help

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120

build: out/tinkerbell ## Build the Tinkerbell binary
build-agent: out/tink-agent ## Build the Tink Agent binary

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
fmt: $(GOIMPORTS_FQP) ## Run go fmt
	go fmt ./...
	$(GOIMPORTS_FQP) -w .

FILE_TO_NOT_INCLUDE_IN_COVERAGE := workflow_grpc.pb.go|workflow.pb.go|zz_generated.deepcopy.go|facility_string.go|severity_string.go

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
	$(GOIMPORTS_FQP) -w .

TINKERBELL_SOURCES := $(shell find $(go list -deps ./cmd/tinkerbell | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

crossbinaries := out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64
out/tinkerbell-linux-amd64: FLAGS=GOARCH=amd64
out/tinkerbell-linux-arm64: FLAGS=GOARCH=arm64
out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64: $(generated_go_files) $(TINKERBELL_SOURCES)
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -tags "${GO_TAGS}" -v -o $@ ./cmd/tinkerbell
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

TINKERBELL_SOURCES := $(shell find $(go list -deps ./cmd/tinkerbell | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

out/tinkerbell: $(generated_go_files) $(TINKERBELL_SOURCES) ## Compile Tinkerbell for the current architecture
	${FLAGS} CGO_ENABLED=0 go build -ldflags="-s -w" -tags "${GO_TAGS}" -v -o $@ ./cmd/tinkerbell
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

cross-compile: $(crossbinaries) ## Compile for all architectures

AGENT_SOURCES := $(shell find $(go list -deps ./cmd/agent | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

crossbinaries-agent := out/tink-agent-linux-amd64 out/tink-agent-linux-arm64
out/tink-agent-linux-amd64: FLAGS=GOARCH=amd64
out/tink-agent-linux-arm64: FLAGS=GOARCH=arm64
out/tink-agent-linux-amd64 out/tink-agent-linux-arm64: $(AGENT_SOURCES)
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -tags "${GO_TAGS}" -ldflags="-s -w" -v -o $@ ./cmd/agent
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

out/tink-agent: $(AGENT_SOURCES) ## Compile Tink Agent for the current architecture
	${FLAGS} CGO_ENABLED=0 go build -ldflags="-s -w" -tags "${GO_TAGS}" -v -o $@ ./cmd/agent
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

cross-compile-agent: $(crossbinaries-agent) ## Compile Tink Agent for all architectures

.PHONY: generate-proto
generate-proto: $(BUF_FQP) $(PROTOC_GEN_GO_GRPC_FQP) $(PROTOC_GEN_GO_FQP) ## Generate code from proto files.
	$(BUF_FQP) generate
	$(MAKE) fmt

# Kubernetes CRD generation
.PHONY: manifests
manifests: $(CONTROLLER_GEN_FQP) ## Generate WebhookConfiguration and CustomResourceDefinition objects.
	$(CONTROLLER_GEN_FQP) crd webhook paths="./..." output:crd:artifacts:config=crd/bases
	$(MAKE) fmt

.PHONY: generate
generate: $(CONTROLLER_GEN_FQP) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN_FQP) object:headerFile="pkg/api/boilerplate.go.txt" paths="./..."
	$(MAKE) fmt

.PHONY: dep-graph
dep-graph: $(GODEPGRAPH_FQP) ## Generate a dependency graph
	rm -rf out/dep-graph.txt out/dep-graph.png
	$(GODEPGRAPH_FQP) -s -novendor -onlyprefixes "github.com/tinkerbell/tinkerbell,./cmd/agent,./cmd/tinkerbell" ./cmd/agent ./cmd/tinkerbell > out/dep-graph.txt
	cat out/dep-graph.txt | dot -Txdot -o out/dep-graph.dot

######### Helm charts - start #########
helm-files := $(shell git ls-files helm/tinkerbell/ | grep -v helm/tinkerbell/docs)
helm-package: out/helm/tinkerbell-$(VERSION).tgz ## Helm chart for Tinkerbell
out/helm/tinkerbell-$(VERSION).tgz: $(helm-files)
	helm package -d out/helm/ helm/tinkerbell --version $(VERSION)

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
.PHONY: prepare-buildx
prepare-buildx: ## Prepare the buildx environment.
## the "|| true" is to avoid failing if the builder already exists.
	docker buildx create --name tinkerbell-multiarch --use --driver docker-container || true

.PHONY: image
image: cross-compile ## Build the Tinkerbell container image
	docker build -t $(IMAGE_NAME) -f Dockerfile.tinkerbell .

.PHONY: build-push-image
build-push-image: ## Build and push the container image for both Amd64 and Arm64 architectures.
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(IMAGE_NAME):$(GIT_COMMIT) -t $(IMAGE_NAME):latest -f Dockerfile.tinkerbell .

.PHONY: image-agent
image-agent: cross-compile-agent ## Build the Tink Agent container image
	docker build -t $(IMAGE_NAME_AGENT) -f Dockerfile.agent .

.PHONY: build-push-image-agent
build-push-image-agent: ## Build and push the container image for both Amd64 and Arm64 architectures.
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(IMAGE_NAME_AGENT):$(GIT_COMMIT) -t $(IMAGE_NAME_AGENT):latest -f Dockerfile.agent .

######### Build container images - end   #########

.PHONY: clean
clean: ## Remove all cross compiled Tinkerbell binaries
	rm -f out/tinkerbell out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64

.PHONY: clean-agent
clean-agent: ## Remove all cross compiled Tink Agent binaries
	rm -f out/tink-agent out/tink-agent-linux-amd64 out/tink-agent-linux-arm64

.PHONY: clean-tools
clean-tools: ## Remove all tools
	rm -rf $(TOOLS_DIR)

.PHONY: clean-all
clean-all: clean clean-agent clean-tools ## Remove all binaries and tools

############## Tools ##############
$(GOIMPORTS_FQP):
	GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@$(GOIMPORT_VER)
	@mv $(TOOLS_DIR)/goimports $(GOIMPORTS_FQP)

$(CONTROLLER_GEN_FQP):
	GOBIN=$(TOOLS_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	@mv $(TOOLS_DIR)/controller-gen $(CONTROLLER_GEN_FQP)

$(PROTOC_GEN_GO_GRPC_FQP):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC_FQP)

$(PROTOC_GEN_GO_FQP):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go $(PROTOC_GEN_GO_FQP)

$(BUF_FQP):
	GOBIN=$(TOOLS_DIR) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@mv $(TOOLS_DIR)/buf $(BUF_FQP)

$(UPX_FQP):
	mkdir -p $(TOOLS_DIR)
	(cd $(TOOLS_DIR); curl -sSfLO https://github.com/upx/upx/releases/download/v$(UPX_VER)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	(cd $(TOOLS_DIR); tar -xvf upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux.tar.xz)
	@mv $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux/upx $(UPX_FQP)
	@rm -rf $(TOOLS_DIR)/upx-$(UPX_VER)-$(LOCAL_ARCH_ALT)_linux*

$(GODEPGRAPH_FQP):
	GOBIN=$(TOOLS_DIR) go install github.com/kisielk/godepgraph@$(GODEPGRAPH_VER)
	@mv $(TOOLS_DIR)/godepgraph $(GODEPGRAPH_FQP)

.PHONY: tools
tools: $(GOIMPORTS_FQP) $(CONTROLLER_GEN_FQP) $(PROTOC_GEN_GO_GRPC_FQP) $(PROTOC_GEN_GO_FQP) $(BUF_FQP) $(UPX_FQP) $(GODEPGRAPH_FQP) ## Install all tools

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
GOLANGCI_LINT_VERSION ?= v1.64.5
GOLANGCI_LINT_BIN := $(LINT_ROOT)/out/linters/golangci-lint-$(GOLANGCI_LINT_VERSION)-$(LINT_ARCH)
$(GOLANGCI_LINT_BIN):
	mkdir -p $(LINT_ROOT)/out/linters
	rm -rf $(LINT_ROOT)/out/linters/golangci-lint-*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LINT_ROOT)/out/linters $(GOLANGCI_LINT_VERSION)
	mv $(LINT_ROOT)/out/linters/golangci-lint $@

LINTERS += golangci-lint-lint
golangci-lint-lint: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -execdir sh -c '"$(GOLANGCI_LINT_BIN)" run --timeout 10m -c "$(GOLANGCI_LINT_CONFIG)"' '{}' '+'

FIXERS += golangci-lint-fix
golangci-lint-fix: $(GOLANGCI_LINT_BIN)
	find . -name go.mod -execdir "$(GOLANGCI_LINT_BIN)" run -c "$(GOLANGCI_LINT_CONFIG)" --fix \;

.PHONY: _lint $(LINTERS)
_lint: $(LINTERS)

.PHONY: fix $(FIXERS)
fix: $(FIXERS)
