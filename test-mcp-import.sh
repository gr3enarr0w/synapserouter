#!/bin/bash

echo "Testing MCP server import..."
echo ""

# Test the exact command from config.go
PYTHONPATH="/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/mcp-servers/context-persistence/src" \
/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/mcp-servers/context-persistence/venv3.12/bin/python3 \
-m context_persistence.server --help 2>&1

echo ""
echo "Exit code: $?"