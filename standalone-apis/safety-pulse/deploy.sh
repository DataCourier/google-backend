#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/../.."
API_BASE="https://safety-pulse-133184350530.us-central1.run.app"

# --- Backend ---
echo "=== Deploying backend to Cloud Run ==="
cd "$REPO_ROOT"
make deploy-safety

echo ""
echo "=== Done! ==="
echo "Backend: $API_BASE"

# --- Frontend ---
# TODO: Set up custom domain with Cloudflare and deploy frontend
# (skipping 418e.com — safety-pulse will use its own domain)
