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

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[%\/0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

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
SETUP_ENVTEST 			:= go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
ENVTEST_BIN_DIR 		:= $(shell pwd)/bin

# The kubernetes version to use with envtest. Overridable when invoking make.
# E.g. make ENVTEST_KUBE_VERSION=1.24 test-integration
ENVTEST_KUBE_VERSION 	?= 1.25

.PHONY: setup-envtest
setup-envtest:
	@echo Installing Kubernetes $(ENVTEST_KUBE_VERSION) binaries into $(ENVTEST_BIN_DIR); \
	$(SETUP_ENVTEST) use --bin-dir $(ENVTEST_BIN_DIR) $(ENVTEST_KUBE_VERSION)

# Integration tests are located next to unit test. This recipe will search the code base for
# files including the "//go:build integration" build tag and build them into the test binary.
# For packages containing both unit and integration tests its recommended to populate 
# "//go:build !integration" in all unit test sources so as to avoid compiling them in this recipe.
.PHONY: test-integration
test-integration: setup-envtest
test-integration: TEST_DIRS := $(shell grep -R --include="*.go" -l -E "//go:build.*\sintegration" . | xargs dirname | uniq)
test-integration: ## Run integration tests.
	source <($(SETUP_ENVTEST) use -p env --bin-dir $(ENVTEST_BIN_DIR) $(ENVTEST_KUBE_VERSION)); \
	go test $(GO_TEST_ARGS) -tags=integration -coverprofile=coverage.out $(TEST_DIRS)

# When we build the image its Linux based. This means we need a Linux binary hence we need to export
# GOOS so we have compatible binary.
.PHONY: image
image: export GOOS=linux
image: build ## Build a Linux based Hegel image for the the host architecture.
	DOCKER_BUILDKIT=1 docker build $(IMAGE_ARGS) .

# The command to run mockgen.
MOCKGEN = go run github.com/golang/mock/mockgen@v1.6

mocks: ## Generate mocks for testing.
	$(MOCKGEN) \
		-destination internal/frontend/ec2/frontend_mock_test.go \
		-package ec2 \
		-source internal/frontend/ec2/frontend.go
	$(MOCKGEN) \
		-destination internal/backend/kubernetes/backend_mock_test.go \
		-package kubernetes \
		-source internal/backend/kubernetes/backend.go

OUT_DIR 	?= $(shell pwd)/out
BIN_DIR		?= $(OUT_DIR)/bin
LINT_DIR 	?= $(OUT_DIR)/linters

GOLANGCI_LINT_VERSION 	?= v1.50.1
GOLANGCI_LINT 			:= go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

HADOLINT_VERSION 	?= v2.12.0
HADOLINT_TARGET 	:= install/hadolint-$(HADOLINT_VERSION)
HADOLINT 			:= $(LINT_DIR)/hadolint-$(HADOLINT_VERSION)

YAMLLINT_VERSION 	?= v1.28.0
YAMLLINT_TARGET 	:= install/yamllint-$(YAMLLINT_VERSION)
YAMLLINT_INSTALL 	:= $(LINT_DIR)/yamllint-$(YAMLLINT_VERSION)
# We install yamllint into a target directory to avoid installing something to the users system.
# This makes it necessary to set the PYTHONPATH so yamllint can import its modules.
YAMLLINT 			:= PYTHONPATH=$(YAMLLINT_INSTALL) $(YAMLLINT_INSTALL)/bin/yamllint

LINT_OS 	:= $(shell uname)
LINT_ARCH 	:= $(shell uname -m)

.PHONY: lint
lint: SHELL := bash
lint: $(shell mkdir -p $(LINT_DIR))
lint: $(HADOLINT_TARGET) $(YAMLLINT_TARGET) ## Run linters.
	$(GOLANGCI_LINT) run
	$(HADOLINT) --no-fail $(shell find . -name "*Dockerfile")
	$(YAMLLINT) .

.PHONY: $(HADOLINT_TARGET)
$(HADOLINT_TARGET):
	curl -sfL https://github.com/hadolint/hadolint/releases/download/$(HADOLINT_VERSION)/hadolint-$(LINT_OS)-$(LINT_ARCH) > $(HADOLINT);\
	chmod u+x $(HADOLINT)

# For simplicity, depend on pip. Its common enough to be present on most systems.
.PHONY: $(YAMLLINT_TARGET)
$(YAMLLINT_TARGET): $(shell mkdir -p $(YAMLLINT_INSTALL))
	python3 -m pip install -t $(YAMLLINT_INSTALL) -qq yamllint==$(YAMLLINT_VERSION)

