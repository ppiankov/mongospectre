BINARY   := mongospectre
MODULE   := github.com/ppiankov/mongospectre
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: build test lint fmt vet clean deps

build:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)/

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run --timeout=5m ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/

deps:
	go mod download
