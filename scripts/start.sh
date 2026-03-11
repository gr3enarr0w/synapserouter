#!/bin/bash

# Start Synapse Router
# Usage: ./scripts/start.sh

set -e

echo "=========================================="
echo "Starting Synapse Router"
echo "=========================================="
echo ""

# Check if .env exists
if [ -f .env ]; then
    echo "Loading environment from .env file..."
    export $(cat .env | grep -v '^#' | xargs)
else
    echo "⚠ No .env file found. Using environment variables."
fi

# Validate required environment variables
if [ -z "$NANOGPT_API_KEY" ]; then
    echo "❌ ERROR: NANOGPT_API_KEY not set"
    echo "Please set your NanoGPT API key:"
    echo "  export NANOGPT_API_KEY='your-key'"
    echo "Or create a .env file from .env.example"
    exit 1
fi

# Create database directory
mkdir -p ~/.mcp/proxy

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ ERROR: Go is not installed"
    echo "Please install Go 1.21 or later"
    exit 1
fi

# Build or run
if [ "$1" == "--build" ]; then
    echo "Building synroute..."
    go build -o synroute main.go
    echo "✓ Build complete"
    echo ""
    echo "Starting server..."
    ./synroute
else
    echo "Running in development mode..."
    go run main.go
fi
