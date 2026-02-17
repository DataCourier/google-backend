#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="$SCRIPT_DIR/frontend"
SITE_DIR="$HOME/dev/418e.com"
API_BASE="https://focus-tube-133184350530.us-central1.run.app"

echo "Building frontend..."
cd "$FRONTEND_DIR"
VITE_BASE=/yt/ VITE_API_BASE="$API_BASE" npm run build

echo "Copying to 418e.com/yt/..."
rm -rf "$SITE_DIR/yt"
cp -r "$FRONTEND_DIR/dist" "$SITE_DIR/yt"

echo "Deploying..."
cd "$SITE_DIR"
git add yt/ _redirects
git commit -m "Update FocusTube frontend"
git push

echo "Done! Live at https://418e.com/yt/"
