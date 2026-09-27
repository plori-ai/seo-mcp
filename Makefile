GO ?= go
GOLANGCI_LINT ?= golangci-lint
VERSION ?= dev
BIN ?= /tmp/seo-mcp

.PHONY: build install test vet lint check fmt

build:
	$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o "$(BIN)" ./cmd/seo-mcp

install:
	$(GO) install -ldflags "-X main.version=$(VERSION)" ./cmd/seo-mcp

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

check: vet test lint

fmt:
	gofmt -w dataforseo/*.go seo/*.go toolset/*.go cmd/seo-mcp/*.go
