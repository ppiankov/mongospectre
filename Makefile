BINARY   := mongospectre
MODULE   := github.com/ppiankov/mongospectre
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test test-integration test-cli-integration lint fmt vet clean deps install coverage coverage-html bench audit check compare docker docker-up docker-down help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: ## Build binary to bin/
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)/

test: ## Run tests with race detector
	go test -race -count=1 ./...

test-integration: ## Run integration tests (requires Docker)
	go test -race -tags=integration -count=1 -timeout=120s ./internal/mongo/

test-cli-integration: ## Run CLI integration tests (requires MONGODB_TEST_URI)
	go test -race -tags=integration -count=1 -timeout=120s ./internal/cli/

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

bench: ## Run benchmarks
	go test -bench=. -benchmem -count=1 -run=^$$ ./internal/...

tidy: ## Verify go.mod is tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

audit: build ## Audit a MongoDB cluster (URI=mongodb://...)
	./bin/$(BINARY) audit --uri $(URI)

check: build ## Check code vs cluster (URI=mongodb://... REPO=./app)
	./bin/$(BINARY) check --uri $(URI) --repo $(REPO)

compare: build ## Compare two clusters (SOURCE=mongodb://... TARGET=mongodb://...)
	./bin/$(BINARY) compare --source $(SOURCE) --target $(TARGET)

docker: ## Build Docker image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t $(BINARY):$(VERSION) .

docker-up: ## Start mongospectre + mongo:7 via docker compose
	docker compose up -d

docker-down: ## Stop docker compose services
	docker compose down
