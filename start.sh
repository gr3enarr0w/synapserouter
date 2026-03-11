#!/bin/bash
set -e

# Synapse Router Startup Script
echo "🚀 Starting Synapse Router..."

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Check if .env file exists, if not create from example
if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        echo "⚠️  No .env file found. Creating from .env.example..."
        cp .env.example .env
        echo "📝 Please edit .env with your actual API keys before continuing"
        exit 1
    else
        echo "❌ No .env or .env.example file found!"
        exit 1
    fi
fi

# Load environment variables
echo "📋 Loading environment variables..."
set -a
source .env
set +a

# Check for required API keys
MISSING_KEYS=()
if [ -z "$SYNROUTE_ANTHROPIC_API_KEY" ] || [ "$SYNROUTE_ANTHROPIC_API_KEY" = "your-anthropic-api-key" ]; then
    MISSING_KEYS+=("SYNROUTE_ANTHROPIC_API_KEY")
fi
if [ -z "$SYNROUTE_OPENAI_API_KEY" ] || [ "$SYNROUTE_OPENAI_API_KEY" = "your-openai-api-key" ]; then
    MISSING_KEYS+=("SYNROUTE_OPENAI_API_KEY")
fi

if [ ${#MISSING_KEYS[@]} -gt 0 ]; then
    echo "⚠️  Warning: The following API keys are not configured:"
    for key in "${MISSING_KEYS[@]}"; do
        echo "   - $key"
    done
    echo ""
    echo "The router will still start, but these providers will not be available."
    echo "Edit .env to add your API keys."
    echo ""
fi

# Ensure database directory exists
DB_PATH="${DB_PATH:-~/.mcp/proxy/usage.db}"
DB_PATH="${DB_PATH/#\~/$HOME}"
DB_DIR=$(dirname "$DB_PATH")
if [ ! -d "$DB_DIR" ]; then
    echo "📁 Creating database directory: $DB_DIR"
    mkdir -p "$DB_DIR"
fi

# Build if binary doesn't exist or source is newer
BINARY="./synroute"
if [ ! -f "$BINARY" ] || [ "$(find . -name '*.go' -newer "$BINARY" | head -1)" ]; then
    echo "🔨 Building synroute..."
    go build -o synroute .
else
    echo "✓ Binary is up to date"
fi

# Set default port if not specified
PORT="${PORT:-8090}"

echo ""
echo "✓ Configuration:"
echo "  Port: $PORT"
echo "  Database: $DB_PATH"
echo ""

# Run the router
echo "🚀 Starting Synapse Router on port $PORT..."
echo ""
exec ./synroute
