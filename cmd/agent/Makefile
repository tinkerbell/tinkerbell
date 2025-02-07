# Configure the Make shell for recipe invocations.
SHELL := bash

# Root output directory.
OUT_DIR ?= $(shell pwd)/out

# Linter installation directory.
TOOLS_DIR ?= $(OUT_DIR)/tools

GIT_COMMIT := $(shell git rev-parse --short HEAD)

NAME ?= tink-agent

CONTAINER_REPOSITORY := ghcr.io/jacobweinstock

IMAGE_NAME := $(CONTAINER_REPOSITORY)/$(NAME)

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[%\/0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# -- Tooling

GOLANGCI_LINT_VERSION 	?= v1.61.0
GOLANGCI_LINT 			:= $(TOOLS_DIR)/golangci-lint
$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(TOOLS_DIR) $(GOLANGCI_LINT_VERSION)

.PHONY: tools
tools: $(GOLANGCI_LINT) ## Install tools

.PHONY: clean-tools
clean-tools: ## Remove tools installed for development.
	rm -rf $(TOOLS_DIR)

.PHONY: lint
lint: ## Run linters.
lint: $(shell mkdir -p $(TOOLS_DIR))
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

.PHONY: binary
binary: ## Build the binary.
	CGO_ENABLED=0 go build -o $(NAME) .

.PHONY: image
image: ## Build the docker image.
	docker build -t $(NAME) .

.PHONY: test
test: ## Run tests.
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ./...

.PHONY: coverage
coverage: test ## Show test coverage
	go tool cover -func=coverage.txt

.PHONY: prepare-buildx
prepare-buildx: ## Prepare the buildx environment.
## the "|| true" is to avoid failing if the builder already exists.
	docker buildx create --name tinkerbell-multiarch --use --driver docker-container || true

.PHONY: clean-buildx
clean-buildx: ## Clean the buildx environment.
	docker buildx rm tinkerbell-multiarch || true

.PHONY: build-push-image
build-push-image: ## Build and push the container image for both Amd64 and Arm64 architectures.
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(IMAGE_NAME):$(GIT_COMMIT) -t $(IMAGE_NAME):latest .