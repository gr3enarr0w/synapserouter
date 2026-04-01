#!/bin/bash
cd /Users/ceverson/Development/synapserouter

echo "=== Starting MCP server ==="
./synroute mcp-serve --addr :9091 &
MCP_PID=$!
sleep 3

echo ""
echo "=== Tools List ==="
curl -s -X POST localhost:9091/mcp/tools/list \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | python3 -m json.tool 2>/dev/null | head -30

echo ""
echo "=== Bash Tool Call ==="
curl -s -X POST localhost:9091/mcp/tools/call \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"bash","arguments":{"command":"echo hello from mcp"}}}' | python3 -m json.tool 2>/dev/null

echo ""
echo "=== Error: Nonexistent Tool ==="
curl -s -X POST localhost:9091/mcp/tools/call \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}' | python3 -m json.tool 2>/dev/null

echo ""
echo "=== Cleanup ==="
kill $MCP_PID 2>/dev/null
echo "done"
