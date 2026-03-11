.PHONY: build test clean run start dev help install deps

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=synroute
GO_FILES=$(shell find . -name '*.go' -type f)
DB_PATH?=~/.mcp/proxy/usage.db

## build: Build the synroute binary
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "✓ Build complete: ./$(BINARY_NAME)"

## build-cli: Build the CLI binary
build-cli:
	@echo "🔨 Building synroute-cli..."
	@cd cmd/synroute-cli && go build -o ../../synroute-cli .
	@echo "✓ CLI build complete: ./synroute-cli"

## build-all: Build both server and CLI
build-all: build build-cli
	@echo "✓ All binaries built"

## test: Run all tests
test:
	@echo "🧪 Running tests..."
	@go test ./... -v

## test-short: Run tests without verbose output
test-short:
	@echo "🧪 Running tests..."
	@go test ./...

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
