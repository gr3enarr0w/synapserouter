#!/bin/bash

# Test script to verify model validation fixes
# Requires synapse-router running on localhost:8080

set -e

echo "======================================"
echo "Model Validation Fix Verification"
echo "======================================"
echo ""

BASE_URL="http://localhost:8080"

# Test 1: Invalid model should return 400
echo "Test 1: Invalid model should return 400"
echo "Request: POST /v1/chat/completions with model='not-a-real-model'"
response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"not-a-real-model","messages":[{"role":"user","content":"test"}]}')
status_code=$(echo "$response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTP_CODE:[0-9]*//g')

if [ "$status_code" = "400" ]; then
  echo "✅ PASS: Got 400 as expected"
  echo "   Error: $(echo "$body" | grep -o '"unknown model"' || echo "$body")"
else
  echo "❌ FAIL: Expected 400, got $status_code"
  echo "   Response: $body"
fi
echo ""

# Test 2: Wrong provider/model combo should return 400
echo "Test 2: Codex provider with Claude model should return 400"
echo "Request: POST /api/provider/codex/v1/chat/completions with model='claude-sonnet-4-5-20250929'"
response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/api/provider/codex/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"test"}]}')
status_code=$(echo "$response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTP_CODE:[0-9]*//g')

if [ "$status_code" = "400" ]; then
  echo "✅ PASS: Got 400 as expected"
  echo "   Error: $(echo "$body" | grep -o 'not compatible' || echo "$body" | head -c 100)"
else
  echo "❌ FAIL: Expected 400, got $status_code"
  echo "   Response: $body"
fi
echo ""

# Test 3: Valid model should work
echo "Test 3: Valid Codex model on Codex route should return 200"
echo "Request: POST /api/provider/codex/v1/chat/completions with model='gpt-5.3-codex'"
response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/api/provider/codex/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"test"}]}')
status_code=$(echo "$response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)

if [ "$status_code" = "200" ] || [ "$status_code" = "503" ]; then
  echo "✅ PASS: Got $status_code (200=working, 503=provider unavailable but validation passed)"
else
  echo "❌ FAIL: Expected 200 or 503, got $status_code"
fi
echo ""

# Test 4: Auto model should work
echo "Test 4: Auto model should work on any provider"
echo "Request: POST /api/provider/codex/v1/chat/completions with model='auto'"
response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/api/provider/codex/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","messages":[{"role":"user","content":"test"}]}')
status_code=$(echo "$response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)

if [ "$status_code" = "200" ] || [ "$status_code" = "503" ]; then
  echo "✅ PASS: Got $status_code (200=working, 503=provider unavailable but validation passed)"
else
  echo "❌ FAIL: Expected 200 or 503, got $status_code"
fi
echo ""

echo "======================================"
echo "Model Validation Tests Complete"
echo "======================================"
