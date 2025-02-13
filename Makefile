# Treat the whole recipe as a one shell script/invocation instead of one-per-line
.ONESHELL:
# Use bash instead of plain sh
SHELL := bash
.SHELLFLAGS := -o pipefail -euc

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

all: help

-include build/tools.mk
-include build/lint.mk

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120

build: out/tinkerbell ## Build the Tinkerbell binary
build-agent: out/tink-agent ## Build the Tink Agent binary

.PHONY: test
test: ## Run go test
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: $(GOIMPORTS_FQP) ## Run go fmt
	go fmt ./...
	$(GOIMPORTS_FQP) -w .

.PHONY: coverage
coverage: test ## Show test coverage
	go tool cover -func=coverage.txt

.PHONY: ci-checks
ci-checks: .github/workflows/ci-checks.sh ## Run the ci-checks.sh script
	./.github/workflows/ci-checks.sh

.PHONY: ci
ci: ci-checks coverage lint vet ## Runs all the same validations and tests that run in CI

TINKERBELL_SOURCES := $(shell find $(go list -deps ./cmd/tinkerbell | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

crossbinaries := out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64
out/tinkerbell-linux-amd64: FLAGS=GOARCH=amd64
out/tinkerbell-linux-arm64: FLAGS=GOARCH=arm64
out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64: $(TINKERBELL_SOURCES)
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/tinkerbell
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

TINKERBELL_SOURCES := $(shell find $(go list -deps ./cmd/tinkerbell | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

out/tinkerbell: $(TINKERBELL_SOURCES) ## Compile Tinkerbell for the current architecture
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/tinkerbell
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

cross-compile: $(crossbinaries) ## Compile for all architectures

AGENT_SOURCES := $(shell find $(go list -deps ./cmd/agent | grep -i tinkerbell | cut -d"/" -f 4-) -type f -name '*.go')

crossbinaries-agent := out/tink-agent-linux-amd64 out/tink-agent-linux-arm64
out/tink-agent-linux-amd64: FLAGS=GOARCH=amd64
out/tink-agent-linux-arm64: FLAGS=GOARCH=arm64
out/tink-agent-linux-amd64 out/tink-agent-linux-arm64: $(AGENT_SOURCES)
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/agent
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

out/tink-agent: $(AGENT_SOURCES) ## Compile Tink Agent for the current architecture
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o $@ ./cmd/agent
	if [ "${COMPRESS}" = "true" ]; then $(MAKE) $(UPX_FQP) && $(UPX_FQP) --best --lzma $@; fi

cross-compile-agent: $(crossbinaries-agent) ## Compile Tink Agent for all architectures

.PHONY: generate-proto
generate-proto: $(BUF_FQP) $(PROTOC_GEN_GO_GRPC_FQP) $(PROTOC_GEN_GO_FQP) ## Generate code from proto files.
	$(BUF_FQP) generate
	$(MAKE) fmt

# Kubernetes CRD generation
.PHONY: manifests
manifests: $(CONTROLLER_GEN_FQP) ## Generate WebhookConfiguration and CustomResourceDefinition objects.
	$(CONTROLLER_GEN_FQP) crd webhook paths="./..." output:crd:artifacts:config=build/config/crd/bases
	$(MAKE) fmt

.PHONY: generate
generate: $(CONTROLLER_GEN_FQP) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN_FQP) object:headerFile="build/config/boilerplate.go.txt" paths="./..."
	$(MAKE) fmt

.PHONY: dep-graph
dep-graph: $(GODEPGRAPH_FQP) ## Generate a dependency graph
	rm -rf out/dep-graph.txt out/dep-graph.png
	$(GODEPGRAPH_FQP) -s -novendor -horizontal -onlyprefixes "github.com/tinkerbell/tinkerbell,./cmd/agent,./cmd/tinkerbell" ./cmd/agent ./cmd/tinkerbell > out/dep-graph.txt
	cat out/dep-graph.txt | dot -Tpng -Goverlap=scale -Gsplines=true -o out/dep-graph.png

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
