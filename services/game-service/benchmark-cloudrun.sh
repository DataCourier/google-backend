#!/bin/bash
# Cloud Run cold start benchmark
# Measures actual cold start time on Google Cloud Run

set -e

SERVICE_NAME="game-service"
REGION="us-central1"

echo "================================================"
echo "  Cloud Run Cold Start Benchmark"
echo "================================================"
echo ""

# Get service URL
SERVICE_URL=$(gcloud run services describe $SERVICE_NAME --region $REGION --format="value(status.url)" 2>/dev/null)

if [ -z "$SERVICE_URL" ]; then
    echo "ERROR: Service $SERVICE_NAME not found in $REGION"
    echo "Deploy first with: make deploy-game"
    exit 1
fi

echo "Service: $SERVICE_NAME"
echo "URL: $SERVICE_URL"
echo ""

# Check current instance count
echo "1. Checking current state..."
INSTANCES=$(gcloud run services describe $SERVICE_NAME --region $REGION \
    --format="value(spec.template.spec.containerConcurrency)" 2>/dev/null || echo "unknown")
MIN_INSTANCES=$(gcloud run services describe $SERVICE_NAME --region $REGION \
    --format="value(spec.template.metadata.annotations.'autoscaling.knative.dev/minScale')" 2>/dev/null || echo "0")

echo "   Min instances: ${MIN_INSTANCES:-0}"
echo ""

# Ensure min-instances is 0 for cold start test
if [ "$MIN_INSTANCES" != "0" ] && [ -n "$MIN_INSTANCES" ]; then
    echo "2. Setting min-instances to 0 for cold start test..."
    gcloud run services update $SERVICE_NAME --region $REGION \
        --min-instances=0 --quiet 2>/dev/null
    echo "   Done. Waiting 60s for scale-down..."
    sleep 60
else
    echo "2. Min instances already 0. Waiting 30s to ensure scale-down..."
    sleep 30
fi

# Warm request first (to establish baseline after cold)
echo ""
echo "3. Warming up (this may be a cold start)..."
WARM_START=$(date +%s%3N)
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$SERVICE_URL/")
WARM_END=$(date +%s%3N)
WARM_TIME=$((WARM_END - WARM_START))
echo "   Warm-up request: ${WARM_TIME}ms (HTTP $HTTP_CODE)"

# Wait for scale down again
echo ""
echo "4. Waiting 90s for scale to zero..."
sleep 90

# Cold start measurement
echo ""
echo "5. Cold start measurement..."

# Use curl with timing
COLD_START=$(date +%s%3N)
TIMING=$(curl -s -o /dev/null -w "dns:%{time_namelookup} connect:%{time_connect} ttfb:%{time_starttransfer} total:%{time_total}" "$SERVICE_URL/")
COLD_END=$(date +%s%3N)
COLD_TIME=$((COLD_END - COLD_START))

echo "   Total cold start: ${COLD_TIME}ms"
echo "   Breakdown: $TIMING"

# Test API endpoint
echo ""
echo "6. API endpoint cold start..."
sleep 90

API_START=$(date +%s%3N)
curl -s "$SERVICE_URL/buckets/mine/notes" -H "Authorization: local:bench" > /dev/null
API_END=$(date +%s%3N)
API_TIME=$((API_END - API_START))

echo "   GET /buckets/mine/notes: ${API_TIME}ms"

# Multiple requests to see warm performance
echo ""
echo "7. Warm request performance (10 requests)..."
TOTAL=0
for i in {1..10}; do
    REQ_START=$(date +%s%3N)
    curl -s "$SERVICE_URL/" > /dev/null
    REQ_END=$(date +%s%3N)
    REQ_TIME=$((REQ_END - REQ_START))
    TOTAL=$((TOTAL + REQ_TIME))
    echo "   Request $i: ${REQ_TIME}ms"
done
AVG=$((TOTAL / 10))

echo ""
echo "================================================"
echo "  Summary"
echo "================================================"
echo "  Cold start (home):     ${COLD_TIME}ms"
echo "  Cold start (API):      ${API_TIME}ms"
echo "  Warm avg (10 reqs):    ${AVG}ms"
echo "================================================"
