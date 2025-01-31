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

test: ## Run go test
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ./...

vet: ## Run go vet
	go vet ./...

coverage: test ## Show test coverage
	go tool cover -func=coverage.txt

ci: coverage lint vet ## Runs all the same validations and tests that run in CI

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120

crossbinaries := out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64
out/tinkerbell-linux-amd64: FLAGS=GOARCH=amd64
out/tinkerbell-linux-arm64: FLAGS=GOARCH=arm64
out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64: cleanup
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -v -o $@ .

out/tinkerbell: cleanup
	${FLAGS} CGO_ENABLED=0 GOOS=linux go build -v -o $@ .

crosscompile: $(crossbinaries) ## Compile for all architectures

cleanup:
	rm -f out/tinkerbell out/tinkerbell-linux-amd64 out/tinkerbell-linux-arm64
