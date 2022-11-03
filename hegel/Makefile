GOARCH ?= $(shell go env GOARCH)
GOOS ?= $(shell go env GOOS)

# The image recipe calls build hence build doesn't feature here.
all: unit-test image

.PHONY: build
build:
	CGO_ENABLED=0 \
	GOOS=$$GOOS \
	GOARCH=$$GOARCH \
	go build \
		-ldflags="-X build.gitRevision=$(shell git rev-parse --short HEAD)" \
		-o hegel-$(GOOS)-$(GOARCH) \
		./cmd/hegel

.PHONY: unit-test
test:
	go test $(GO_TEST_ARGS) -coverprofile=coverage.out ./...

IMAGE_ARGS ?= -t hegel

# When we build the image its Linux based. This means we need a Linux binary hence we need to export
# GOOS so we have compatible binary.
.PHONY: image
image: export GOOS=linux
image: build
	docker build --build-arg GOPROXY=$(GOPROXY) $(IMAGE_ARGS) .

# BEGIN: lint-install --dockerfile=warn .
# http://github.com/tinkerbell/lint-install

GOLINT_VERSION ?= v1.50.1
HADOLINT_VERSION ?= v2.10.0

YAMLLINT_VERSION ?= 1.28.0
LINT_OS := $(shell uname)
LINT_ARCH := $(shell uname -m)
LINT_ROOT := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# shellcheck and hadolint lack arm64 native binaries: rely on x86-64 emulation
ifeq ($(LINT_OS),Darwin)
	ifeq ($(LINT_ARCH),arm64)
		LINT_ARCH=x86_64
	endif
endif


GOLINT_CONFIG = $(LINT_ROOT)/.golangci.yml
YAMLLINT_ROOT = out/linters/yamllint-$(YAMLLINT_VERSION)

lint: out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH) out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH) $(YAMLLINT_ROOT)/bin/yamllint
	out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH) run
	out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH) --no-fail $(shell find . -name "*Dockerfile")
	PYTHONPATH=$(YAMLLINT_ROOT)/lib $(YAMLLINT_ROOT)/bin/yamllint .

fix: out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)
	out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH) run --fix

out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH):
	mkdir -p out/linters
	curl -sfL https://github.com/hadolint/hadolint/releases/download/v2.6.1/hadolint-$(LINT_OS)-$(LINT_ARCH) > out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH)
	chmod u+x out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH)

out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH):
	mkdir -p out/linters
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b out/linters $(GOLINT_VERSION)
	mv out/linters/golangci-lint out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)

$(YAMLLINT_ROOT)/bin/yamllint:
	mkdir -p $(YAMLLINT_ROOT)/lib
	curl -sSfL https://github.com/adrienverge/yamllint/archive/refs/tags/v$(YAMLLINT_VERSION).tar.gz | tar -C out/linters -zxf -
	cd $(YAMLLINT_ROOT) && PYTHONPATH=lib python setup.py -q install --prefix . --install-lib lib
# END: lint-install --dockerfile=warn .
