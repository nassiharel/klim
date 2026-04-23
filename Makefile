VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

MODULE  := github.com/nassiharel/clim
LDFLAGS := -s -w \
  -X $(MODULE)/internal/build.Version=$(VERSION) \
  -X $(MODULE)/internal/build.Commit=$(COMMIT) \
  -X $(MODULE)/internal/build.Date=$(DATE)

.DEFAULT_GOAL := all
.PHONY: all build run test lint tidy vulncheck cover clean marketplace-validate marketplace-assemble help

all: lint test build ## Run lint, test, and build

build: ## Build the clim binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/clim ./cmd/clim

run: ## Run clim from source
	go run -ldflags "$(LDFLAGS)" ./cmd/clim

test: ## Run all tests
	go test -race -count=1 ./...

lint: ## Run golangci-lint
	golangci-lint run

tidy: ## Check go.mod tidiness
	go mod tidy -diff

vulncheck: ## Run govulncheck for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

cover: ## Generate test coverage report
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean: ## Remove build artifacts
	rm -rf bin/ dist/ coverage.out coverage.html

marketplace-validate: ## Validate marketplace tool YAML files
	go run ./internal/marketplace/validate

marketplace-assemble: ## Assemble marketplace.yaml from individual tool files
	go run ./internal/marketplace/assemble -fallback "$$(test -f marketplace.yaml && printf '%s' marketplace.yaml || printf '%s' /dev/null)" -o marketplace.yaml

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":[^:]*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'
