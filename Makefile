BINARY   := mongospectre
MODULE   := github.com/ppiankov/mongospectre
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: build test test-integration lint fmt vet clean deps install coverage coverage-html help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: ## Build binary to bin/
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)/

test: ## Run tests with race detector
	go test -race -count=1 ./...

test-integration: ## Run integration tests (requires Docker)
	go test -race -tags=integration -count=1 -timeout=120s ./internal/mongo/

lint: ## Run golangci-lint
	golangci-lint run --timeout=5m ./...

fmt: ## Format Go source files
	gofmt -w .

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out

deps: ## Download module dependencies
	go mod download

install: ## Install to GOPATH/bin
	go install -ldflags="$(LDFLAGS)" ./cmd/$(BINARY)/

coverage: ## Run tests with coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

coverage-html: coverage ## Open coverage report in browser
	go tool cover -html=coverage.out

tidy: ## Verify go.mod is tidy
	go mod tidy
	git diff --exit-code go.mod go.sum
