#!/bin/bash
# Test user service locally
# Requires: Firestore emulator running on :9099

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "=== Testing User Service ==="
echo "Base URL: $BASE_URL"
echo ""

# Test 1: Get/create user (alice)
echo "1. GET /users/me (alice - first time, creates user)"
curl -s -X GET "$BASE_URL/users/me" \
  -H "Authorization: local:alice:alice@example.com:Alice Smith" \
  | jq .
echo ""

# Test 2: Get user again (should exist now)
echo "2. GET /users/me (alice - already exists)"
curl -s -X GET "$BASE_URL/users/me" \
  -H "Authorization: local:alice" \
  | jq .
echo ""

# Test 3: Update profile
echo "3. PUT /users/me (update bio)"
curl -s -X PUT "$BASE_URL/users/me" \
  -H "Authorization: local:alice" \
  -H "Content-Type: application/json" \
  -d '{"bio": "I love coding!", "location": "San Francisco"}' \
  | jq .
echo ""

# Test 4: Different user (bob)
echo "4. GET /users/me (bob - new user)"
curl -s -X GET "$BASE_URL/users/me" \
  -H "Authorization: local:bob:bob@example.com:Bob Jones" \
  | jq .
echo ""

# Test 5: Get public profile
echo "5. GET /users/alice (public profile)"
curl -s -X GET "$BASE_URL/users/alice" \
  -H "Authorization: local:bob" \
  | jq .
echo ""

# Test 6: No auth
echo "6. GET /users/me (no auth - should fail)"
curl -s -X GET "$BASE_URL/users/me" | jq .
echo ""

echo "=== Done ==="
