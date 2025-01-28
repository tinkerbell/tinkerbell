all: help

-include lint.mk

test: ## Run go test
	CGO_ENABLED=1 go test -race -coverprofile=coverage.txt -covermode=atomic -v ${TEST_ARGS} ./...

vet: ## Run go vet
	go vet ./...

coverage: test ## Show test coverage
	go tool cover -func=coverage.txt

ci: coverage lint vet ## Runs all the same validations and tests that run in CI

help: ## Print this help
	@grep --no-filename -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*##/·/' | sort | column -ts '·' -c 120