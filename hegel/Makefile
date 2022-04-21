binary := cmd/hegel

all: build

.PHONY: ${binary} build
${binary}: 
build:
	CGO_ENABLED=0 GOOS=$$GOOS go build -ldflags="-X build.gitRevision=$(shell git rev-parse --short HEAD)" -o hegel ./cmd/hegel

.PHONY: unit-test
unit-test:
	go test $(GO_TEST_ARGS) -coverprofile=unit-test.coverage ./...

.PHONY: image
image:
	docker build $(IMAGE_ARGS) -f ./cmd/hegel/Dockerfile .

.PHONY: gen
gen: grpc/protos/hegel/hegel.pb.go
grpc/protos/hegel/hegel.pb.go: grpc/protos/hegel/hegel.proto
	protoc --go_out=plugins=grpc:./ grpc/protos/hegel/hegel.proto
	goimports -w $@
ifeq ($(CI),drone)
run: ${binary}
	${binary}
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ${TEST_ARGS} ./...
endif

# BEGIN: lint-install --dockerfile=warn .
# http://github.com/tinkerbell/lint-install

GOLINT_VERSION ?= v1.42.0
HADOLINT_VERSION ?= v2.7.0

YAMLLINT_VERSION ?= 1.26.3
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
