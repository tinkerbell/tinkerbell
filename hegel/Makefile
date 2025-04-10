# Configure the Make shell for recipe invocations.
SHELL := bash

# Specify the target architecture to build the binary for. (Recipes: build, image)
GOARCH ?= $(shell go env GOARCH)

# Specify the target OS to build the binary for. (Recipes: build)
GOOS ?= $(shell go env GOOS)

# Specify the GOPROXYs to use in the build of the binary. (Recipes: build)
GOPROXY ?= $(shell go env GOPROXY)

# Specify additional `docker build` arguments. (Recipes: image)
IMAGE_ARGS ?= -t hegel

# Root output directory.
OUT_DIR ?= $(shell pwd)/out

# Project binary output directory.
BIN_DIR ?= $(OUT_DIR)/bin

# Linter installation directory.
TOOLS_DIR ?= $(OUT_DIR)/tools

# Platform variables
PLATFORM_OS 	?= $(shell uname)
PLATFORM_ARCH 	?= $(shell uname -m)

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[%\/0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# -- Tooling

GOLANGCI_LINT_VERSION 	?= v1.61.0
GOLANGCI_LINT 			:= $(TOOLS_DIR)/golangci-lint
$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(TOOLS_DIR) $(GOLANGCI_LINT_VERSION)

# The command to run mockgen.
MOCKGEN_VERSION 	?= v1.6.0
MOCKGEN 			:= $(TOOLS_DIR)/mockgen
$(MOCKGEN): PLATFORM_ARCH := $(if $(filter x86_64,$(PLATFORM_ARCH)),amd64,$(PLATFORM_ARCH))
$(MOCKGEN): PLATFORM_OS := $(shell echo $(PLATFORM_OS) | tr '[:upper:]' '[:lower:]')
$(MOCKGEN):
	curl -sSfL https://github.com/golang/mock/releases/download/$(MOCKGEN_VERSION)/mock_$(MOCKGEN_VERSION:v%=%)_$(PLATFORM_OS)_$(PLATFORM_ARCH).tar.gz | \
		tar -C $(TOOLS_DIR) --strip-components=1 -xzf -
	@rm $(TOOLS_DIR)/README.md $(TOOLS_DIR)/LICENSE

SETUP_ENVTEST_VERSION 	:= latest
SETUP_ENVTEST 			:= $(TOOLS_DIR)/setup-envtest
$(SETUP_ENVTEST):
	GOBIN=$(TOOLS_DIR) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

HADOLINT_VERSION 	?= v2.12.0
HADOLINT 			:= $(TOOLS_DIR)/hadolint
$(HADOLINT):
	curl -sSfL https://github.com/hadolint/hadolint/releases/download/$(HADOLINT_VERSION)/hadolint-$(PLATFORM_OS)-$(PLATFORM_ARCH) > $(HADOLINT);\
	chmod u+x $(HADOLINT)

YAMLLINT_VERSION 	?= v1.28.0
YAMLTOOLS_DIR 		:= $(TOOLS_DIR)/yamllint
YAMLLINT_BIN		:= $(YAMLTOOLS_DIR)/bin/yamllint
# We install yamllint into a target directory to avoid installing something to the users system.
# This makes it necessary to set the PYTHONPATH so yamllint can import its modules.
YAMLLINT 			:= PYTHONPATH=$(YAMLTOOLS_DIR) $(YAMLLINT_BIN)
# For simplicity, depend on pip. Its common enough to be present on most systems.
$(YAMLLINT_BIN): $(shell mkdir -p $(YAMLTOOLS_DIR))
	python3 -m pip install -t $(YAMLTOOLS_DIR) -qq yamllint==$(YAMLLINT_VERSION)

.PHONY: tools
tools: $(GOLANGCI_LINT) $(MOCKGEN) $(SETUP_ENVTEST) $(HADOLINT) $(YAMLLINT_BIN) ## Install tools required for development.

.PHONY: clean-tools
clean-tools: ## Remove tools installed for development.
	@chmod -R +w $(TOOLS_DIR)/envtest &> /dev/null || true
	rm -rf $(TOOLS_DIR)

# -- Everything else

# The image recipe calls build hence build doesn't feature here.
all: test image ## Run tests and build the Hegel a Linux Hegel image for the host architecture.

.PHONY: build
build: ## Build the Hegel binary. Use GOOS and GOARCH to set the target OS and architecture.
	CGO_ENABLED=0 \
	GOOS=$$GOOS \
	GOARCH=$$GOARCH \
	GOPROXY=$$GOPROXY \
	go build \
		-o hegel-$(GOOS)-$(GOARCH) \
		./cmd/hegel

.PHONY: test
test: ## Run unit tests.
	go test $(GO_TEST_ARGS) -coverprofile=coverage.out ./...

.PHONY: test-e2e
test-e2e: ## Run E2E tests.
	go test $(GO_TEST_ARGS) -tags=e2e -coverprofile=coverage.out ./internal/e2e

# Version should match with whatever we consume in sources (check the go.mod).
ENVTEST_BIN_DIR := $(TOOLS_DIR)/envtest

# The kubernetes version to use with envtest. Overridable when invoking make.
# E.g. make ENVTEST_KUBE_VERSION=1.24 test-integration
ENVTEST_KUBE_VERSION ?= 1.25

.PHONY: setup-envtest
setup-envtest: ## Download and setup binaries for envtest testing.
setup-envtest: $(SETUP_ENVTEST)
	@echo Installing Kubernetes $(ENVTEST_KUBE_VERSION) binaries into $(ENVTEST_BIN_DIR); \
	$(SETUP_ENVTEST) use --bin-dir $(ENVTEST_BIN_DIR) $(ENVTEST_KUBE_VERSION)

# Integration tests are located next to unit test. This recipe will search the code base for
# files including the "//go:build integration" build tag and build them into the test binary.
# For packages containing both unit and integration tests its recommended to populate
# "//go:build !integration" in all unit test sources so as to avoid compiling them in this recipe.
.PHONY: test-integration
test-integration: ## Run integration tests.
test-integration: setup-envtest
test-integration: TEST_DIRS := $(shell grep -R --include="*.go" -l -E "//go:build.*\sintegration" . | xargs dirname | uniq)
test-integration:
	source <($(SETUP_ENVTEST) use -p env --bin-dir $(ENVTEST_BIN_DIR) $(ENVTEST_KUBE_VERSION)); \
	go test $(GO_TEST_ARGS) -tags=integration -coverprofile=coverage.out $(TEST_DIRS)

# When we build the image its Linux based. This means we need a Linux binary hence we need to export
# GOOS so we have compatible binary.
.PHONY: image
image: ## Build a Linux based Hegel image for the the host architecture.
image: export GOOS=linux
image: build
	DOCKER_BUILDKIT=1 docker build $(IMAGE_ARGS) .

mocks: ## Generate mocks for testing.
mocks: $(MOCKGEN)
	$(MOCKGEN) \
		-destination internal/frontend/ec2/frontend_mock_test.go \
		-package ec2 \
		-source internal/frontend/ec2/frontend.go
	$(MOCKGEN) \
		-destination internal/backend/kubernetes/backend_mock_test.go \
		-package kubernetes \
		-source internal/backend/kubernetes/backend.go
	$(MOCKGEN) \
		-destination internal/healthcheck/healthcheck_mock_test.go \
		-package healthcheck \
		-source internal/healthcheck/health_check.go

.PHONY: lint
lint: ## Run linters.
lint: $(shell mkdir -p $(TOOLS_DIR))
lint: $(GOLANGCI_LINT) $(HADOLINT) $(YAMLLINT_BIN)
	$(GOLANGCI_LINT) run
	$(HADOLINT) --no-fail $(shell find . -name "*Dockerfile")
	$(YAMLLINT) .



