#!/bin/bash
# Check for framework upgrades and analyze safety with Claude
# Usage: ./check-upgrade.sh [path-to-your-app-backend]
#
# This script compares your app's framework code against the main
# bucket-framework repo and uses Claude to analyze if it's safe to upgrade.

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

FRAMEWORK_REPO="https://github.com/anthropics/bucket-framework.git"  # Change to your repo
FRAMEWORK_BRANCH="main"

# Get the app backend path
if [ -n "$1" ]; then
    APP_BACKEND="$1"
else
    APP_BACKEND="$(pwd)"
fi

if [ ! -d "$APP_BACKEND/services" ]; then
    echo -e "${RED}ERROR: Not a valid backend directory: $APP_BACKEND${NC}"
    echo "Usage: $0 [path-to-your-app-backend]"
    exit 1
fi

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Framework Upgrade Checker${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""
echo "App backend: $APP_BACKEND"
echo ""

# Create temp directory for framework
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# For now, use local framework as reference (change to git clone for real usage)
FRAMEWORK_DIR="$(cd "$(dirname "$0")" && pwd)"
echo -e "${YELLOW}Using local framework as reference: $FRAMEWORK_DIR${NC}"
echo ""

# Uncomment below for real git-based comparison:
# echo "Fetching latest framework from $FRAMEWORK_REPO..."
# git clone --depth 1 --branch $FRAMEWORK_BRANCH $FRAMEWORK_REPO $TEMP_DIR/framework 2>/dev/null
# FRAMEWORK_DIR="$TEMP_DIR/framework"

# Find the service directory in the app
SERVICE_DIR=$(find "$APP_BACKEND/services" -maxdepth 1 -type d ! -name "services" | head -1)
if [ -z "$SERVICE_DIR" ]; then
    echo -e "${RED}ERROR: No service directory found${NC}"
    exit 1
fi
SERVICE_NAME=$(basename "$SERVICE_DIR")
echo "Found service: $SERVICE_NAME"
echo ""

# Compare framework files
echo -e "${BLUE}Comparing framework files...${NC}"
echo ""

DIFF_OUTPUT="$TEMP_DIR/diff.txt"
CHANGED_FILES=""
CHANGES_FOUND=false

# Files to compare (framework code)
FRAMEWORK_FILES=(
    "buckets/personal.go"
    "buckets/router.go"
    "buckets/sharing.go"
    "buckets/org.go"
    "buckets/types.go"
    "auth/auth.go"
    "auth/magic.go"
    "auth/firebase.go"
    "auth/routes.go"
    "users/user.go"
    "users/friends.go"
    "users/routes.go"
)

for file in "${FRAMEWORK_FILES[@]}"; do
    FRAMEWORK_FILE="$FRAMEWORK_DIR/services/game-service/$file"
    APP_FILE="$SERVICE_DIR/$file"

    if [ ! -f "$FRAMEWORK_FILE" ]; then
        continue
    fi

    if [ ! -f "$APP_FILE" ]; then
        echo -e "  ${YELLOW}MISSING:${NC} $file"
        CHANGED_FILES="$CHANGED_FILES\n- $file (missing in your app)"
        CHANGES_FOUND=true
        continue
    fi

    # Compare (ignoring import paths which will differ)
    DIFF=$(diff -u "$APP_FILE" "$FRAMEWORK_FILE" 2>/dev/null | grep -v "^---\|^+++\|^@@" | grep "^[+-]" | grep -v "yourusername" || true)

    if [ -n "$DIFF" ]; then
        echo -e "  ${YELLOW}CHANGED:${NC} $file"
        CHANGED_FILES="$CHANGED_FILES\n- $file"
        echo "--- $file ---" >> "$DIFF_OUTPUT"
        diff -u "$APP_FILE" "$FRAMEWORK_FILE" >> "$DIFF_OUTPUT" 2>/dev/null || true
        echo "" >> "$DIFF_OUTPUT"
        CHANGES_FOUND=true
    else
        echo -e "  ${GREEN}OK:${NC} $file"
    fi
done

echo ""

if [ "$CHANGES_FOUND" = false ]; then
    echo -e "${GREEN}================================================${NC}"
    echo -e "${GREEN}  All framework files are up to date!${NC}"
    echo -e "${GREEN}================================================${NC}"
    exit 0
fi

# Show summary
echo -e "${YELLOW}================================================${NC}"
echo -e "${YELLOW}  Changes detected in framework files${NC}"
echo -e "${YELLOW}================================================${NC}"
echo -e "$CHANGED_FILES"
echo ""

# Check if Claude CLI is available
if ! command -v claude &> /dev/null; then
    echo -e "${YELLOW}Claude CLI not found. Install it to get AI-powered upgrade analysis.${NC}"
    echo ""
    echo "Full diff saved to: $DIFF_OUTPUT"
    echo "Review manually or install Claude CLI: npm install -g @anthropic-ai/claude-code"

    # Still show the diff
    echo ""
    echo -e "${BLUE}=== DIFF ===${NC}"
    cat "$DIFF_OUTPUT"
    exit 0
fi

# Use Claude to analyze the diff
echo -e "${BLUE}Analyzing changes with Claude...${NC}"
echo ""

ANALYSIS_PROMPT="You are reviewing framework upgrade changes for a Go backend.

The user has a backend app that was created from a framework template. The framework has been updated and we need to determine if it's safe to upgrade.

Here are the files that have changed:
$CHANGED_FILES

Here is the diff (their current code vs latest framework):

$(cat "$DIFF_OUTPUT")

Please analyze:
1. **Safety**: Is it safe to upgrade? Any breaking changes?
2. **New Features**: What new features/fixes are available?
3. **Migration Steps**: What steps should they take to upgrade?
4. **Risk Level**: LOW / MEDIUM / HIGH

Be concise and actionable."

# Run Claude analysis
echo "$ANALYSIS_PROMPT" | claude --print 2>/dev/null || {
    echo -e "${YELLOW}Claude analysis failed. Here's the raw diff:${NC}"
    echo ""
    cat "$DIFF_OUTPUT"
}

echo ""
echo -e "${BLUE}================================================${NC}"
echo "Full diff saved to: $DIFF_OUTPUT"
echo -e "${BLUE}================================================${NC}"
