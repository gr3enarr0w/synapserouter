#!/bin/bash
set -e

echo "🧪 Running Synapse Router Integration Tests"
echo ""

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Check if go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go first."
    exit 1
fi

echo "📦 Installing dependencies..."
go mod download
go mod tidy

echo ""
echo "🔨 Building project..."
go build -o /tmp/synroute-test .

echo ""
echo "🧪 Running unit tests..."
go test ./... -short -v

echo ""
echo "🚀 Running integration tests..."
go test -tags=integration -v ./integration_test.go

echo ""
echo "✅ All tests passed!"
