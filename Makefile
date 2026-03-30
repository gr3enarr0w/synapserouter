.PHONY: build test clean run start dev help install uninstall deps

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=synroute
GO_FILES=$(shell find . -name '*.go' -type f)
DB_PATH?=~/.mcp/proxy/usage.db
INSTALL_DIR?=/usr/local/bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

## build: Build the synroute binary
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "✓ Build complete: ./$(BINARY_NAME)"

## build-cli: Build the CLI binary
build-cli:
	@echo "🔨 Building synroute-cli..."
	@cd cmd/synroute-cli && go build -o ../../synroute-cli .
	@echo "✓ CLI build complete: ./synroute-cli"

## build-all: Build both server and CLI
build-all: build build-cli
	@echo "✓ All binaries built"

## test: Run all tests with race detection
test:
	@echo "🧪 Running tests with race detection..."
	@go test -race ./... -v

## test-short: Run tests without verbose output
test-short:
	@echo "🧪 Running tests..."
	@go test -race ./...

## install: Build and install synroute to /usr/local/bin (or INSTALL_DIR)
install: build
	@echo "📦 Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@install -d $(INSTALL_DIR)
	@install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✓ Installed: $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "  Run 'synroute' from anywhere to start"

## uninstall: Remove synroute from /usr/local/bin
uninstall:
	@echo "🗑  Removing $(INSTALL_DIR)/$(BINARY_NAME)..."
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✓ Uninstalled"

## clean: Remove binary and clean build cache
clean:
	@echo "🧹 Cleaning..."
	@rm -f $(BINARY_NAME)
	@go clean
	@echo "✓ Clean complete"

## run: Build and run synroute
run: build
	@./$(BINARY_NAME)

## start: Run using the start.sh script (recommended)
start:
	@./start.sh

## dev: Run in development mode with live reload (requires entr)
dev:
	@echo "🔄 Running in development mode..."
	@echo "Press Ctrl+C to stop"
	@find . -name '*.go' | entr -r go run .

## install: Install required dependencies
deps:
	@echo "📦 Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies installed"

## vet: Run go vet
vet:
	@go vet ./...

## all: Run vet, test, and build
all: vet test build

## check: Run go vet and format checks
check:
	@echo "🔍 Running checks..."
	@go vet ./...
	@gofmt -l . | (! grep .) || (echo "❌ Some files need formatting. Run 'make fmt'" && exit 1)
	@echo "✓ All checks passed"

## fmt: Format all Go files
fmt:
	@echo "📝 Formatting code..."
	@gofmt -w .
	@echo "✓ Formatting complete"

## db-reset: Reset the database (WARNING: deletes all data)
db-reset:
	@echo "⚠️  Removing database: $(DB_PATH)"
	@rm -f $(shell echo $(DB_PATH) | sed 's|^~|$(HOME)|')
	@echo "✓ Database removed. It will be recreated on next start."

## help: Show this help message
help:
	@echo "Synapse Router - Makefile commands:"
	@echo ""
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
