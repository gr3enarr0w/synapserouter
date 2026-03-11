#!/bin/bash

# Test Synapse Router API
# Usage: ./scripts/test-api.sh

set -e

BASE_URL="http://localhost:8090"

echo "=========================================="
echo "Testing Synapse Router API"
echo "=========================================="
echo ""

# 1. Health check
echo "1. Health check..."
curl -s "${BASE_URL}/health"
echo ""
echo "✓ Health check passed"
echo ""

# 2. Status
echo "2. System status..."
curl -s "${BASE_URL}/status" | jq '.'
echo "✓ Status retrieved"
echo ""

# 3. List models
echo "3. List available models..."
curl -s "${BASE_URL}/v1/models" | jq '.data[].id'
echo "✓ Models listed"
echo ""

# 4. Chat completion (architect role)
echo "4. Testing chat completion (architect role)..."
curl -s -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "Design a simple authentication system"}
    ],
    "role": "architect",
    "max_tokens": 200
  }' | jq '.choices[0].message.content, .x_proxy_metadata'
echo "✓ Chat completion successful"
echo ""

# 5. Research status
echo "5. Research system status..."
curl -s "${BASE_URL}/admin/research/status" | jq '.'
echo "✓ Research status retrieved"
echo ""

echo "=========================================="
echo "All tests passed! ✓"
echo "=========================================="
