# Treat the whole recipe as a one shell script/invocation instead of one-per-line
.ONESHELL:
# Use bash instead of plain sh
SHELL := bash
.SHELLFLAGS := -o pipefail -euc

all: help

-include lint.mk

CGO_ENABLED := 0
export CGO_ENABLED

build: out/tinkerbell ## Build the binary
build-agent: out/tink-agent ## Build the Tink Agent binary

test: ## Run go test
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ./...

vet: ## Run go vet
	go vet ./...

coverage: test ## Show test coverage
	go tool cover -func=coverage.txt

ci-checks: .github/workflows/ci-checks.sh
	./.github/workflows/ci-checks.sh

ci: ci-checks coverage lint vet ## Runs all the same validations and tests that run in CI

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120

crossbinaries := out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64
out/tinkerbell-linux-amd64: FLAGS=GOARCH=amd64
out/tinkerbell-linux-arm64: FLAGS=GOARCH=arm64
out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64: cleanup
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/tinkerbell

out/tinkerbell: cleanup
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/tinkerbell

crosscompile: $(crossbinaries) ## Compile for all architectures

cleanup:
	rm -f out/tinkerbell out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64

crossbinaries-agent := out/tink-agent-linux-amd64 out/tink-agent-linux-arm64
out/tink-agent-linux-amd64: FLAGS=GOARCH=amd64
out/tink-agent-linux-arm64: FLAGS=GOARCH=arm64
out/tink-agent-linux-amd64 out/tink-agent-linux-arm64: cleanup-agent
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/agent

out/tink-agent: cleanup-agent
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/agent

crosscompile-agent: $(crossbinaries-agent) ## Compile Tink Agent for all architectures

cleanup-agent:
	rm -f out/tink-agent out/tink-agent-linux-amd64 out/tink-agent-linux-arm64

# Kubernetes CRD generation
# Define the directory tools are installed to.
TOOLS_DIR := $(PWD)/out/tools

CONTROLLER_GEN_VERSION := v0.17.1

CONTROLLER_GEN = $(TOOLS_DIR)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally.
	GOBIN=$(TOOLS_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

GOIMPORTS = $(TOOLS_DIR)/goimports
.PHONY: goimports
goimports: ## Download goimports locally.
	GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@latest

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(MAKE) fmt

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="config/boilerplate.go.txt" paths="./..."
	$(MAKE) fmt

.PHONY: fmt
fmt: goimports ## Run go fmt against code.
	go fmt ./...
	$(GOIMPORTS) -w .

# Protocol Buffer code generation
BUF_VERSION            := v1.50.0
PROTOC_GEN_GO_GRPC_VER := v1.5.1
PROTOC_GEN_GO_VER      := v1.36.5

$(TOOLS_DIR)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VER):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go-grpc $(TOOLS_DIR)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VER)

$(TOOLS_DIR)/protoc-gen-go-$(PROTOC_GEN_GO_VER):
	GOBIN=$(TOOLS_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VER)
	@mv $(TOOLS_DIR)/protoc-gen-go $(TOOLS_DIR)/protoc-gen-go-$(PROTOC_GEN_GO_VER)


$(TOOLS_DIR)/buf-$(BUF_VERSION):
	GOBIN=$(TOOLS_DIR) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	@mv $(TOOLS_DIR)/buf $(TOOLS_DIR)/buf-$(BUF_VERSION)

.PHONY: generate-proto
generate-proto: $(TOOLS_DIR)/buf-$(BUF_VERSION) $(TOOLS_DIR)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VER) $(TOOLS_DIR)/protoc-gen-go-$(PROTOC_GEN_GO_VER) ## Generate code from proto files.
	$(TOOLS_DIR)/buf-$(BUF_VERSION) generate
	$(MAKE) fmt

.PHONY: clean-tools
clean-tools: ## Remove all tools.
	rm -rf $(TOOLS_DIR)
