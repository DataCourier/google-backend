#!/bin/bash
# Cold start benchmark for game-service
# Measures: build time, startup time, first request latency

set -e

PORT=8099
EMULATOR_HOST="localhost:9099"

echo "================================================"
echo "  Cold Start Benchmark - game-service"
echo "================================================"
echo ""

# Ensure emulator is running
if ! curl -s "http://${EMULATOR_HOST}" > /dev/null 2>&1; then
    echo "ERROR: Firestore emulator not running at $EMULATOR_HOST"
    echo "Run: gcloud emulators firestore start --host-port=$EMULATOR_HOST"
    exit 1
fi

export FIRESTORE_EMULATOR_HOST="$EMULATOR_HOST"
export PORT="$PORT"

# Clean build
echo "1. Clean build..."
rm -f game-service 2>/dev/null || true

BUILD_START=$(date +%s%3N)
go build -o game-service .
BUILD_END=$(date +%s%3N)
BUILD_TIME=$((BUILD_END - BUILD_START))

echo "   Build time: ${BUILD_TIME}ms"
echo ""

# Binary size
BINARY_SIZE=$(ls -lh game-service | awk '{print $5}')
echo "   Binary size: $BINARY_SIZE"
echo ""

# Startup time (time until server responds to health check)
echo "2. Cold start..."

STARTUP_START=$(date +%s%3N)
./game-service &
SERVER_PID=$!

# Wait for server to be ready
MAX_WAIT=30
READY=false
for i in $(seq 1 $MAX_WAIT); do
    if curl -s "http://localhost:$PORT/" > /dev/null 2>&1; then
        READY=true
        break
    fi
    sleep 0.05
done

STARTUP_END=$(date +%s%3N)
STARTUP_TIME=$((STARTUP_END - STARTUP_START))

if [ "$READY" = true ]; then
    echo "   Startup time: ${STARTUP_TIME}ms (until first request succeeds)"
else
    echo "   ERROR: Server did not start within ${MAX_WAIT}s"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

# First request latency
echo ""
echo "3. First request latency..."

REQ_START=$(date +%s%3N)
curl -s "http://localhost:$PORT/" > /dev/null
REQ_END=$(date +%s%3N)
REQ_TIME=$((REQ_END - REQ_START))

echo "   GET / : ${REQ_TIME}ms"

# Auth endpoint (more representative of API)
REQ_START=$(date +%s%3N)
curl -s -X POST "http://localhost:$PORT/auth/magic-link" \
    -H "Content-Type: application/json" \
    -d '{"email":"bench@test.com"}' > /dev/null
REQ_END=$(date +%s%3N)
REQ_TIME=$((REQ_END - REQ_START))

echo "   POST /auth/magic-link : ${REQ_TIME}ms"

# Bucket endpoint (authenticated)
REQ_START=$(date +%s%3N)
curl -s "http://localhost:$PORT/buckets/mine/notes" \
    -H "Authorization: local:bench-user" > /dev/null
REQ_END=$(date +%s%3N)
REQ_TIME=$((REQ_END - REQ_START))

echo "   GET /buckets/mine/notes : ${REQ_TIME}ms"

# Cleanup
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "================================================"
echo "  Summary"
echo "================================================"
echo "  Build:   ${BUILD_TIME}ms"
echo "  Startup: ${STARTUP_TIME}ms"
echo "  Binary:  $BINARY_SIZE"
echo "================================================"
