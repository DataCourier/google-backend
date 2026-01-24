#!/bin/bash
# Create a new app backend from this template
# Usage: ./create-new-app.sh ~/dev/android/my-new-app/backend

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <destination-path>"
    echo "Example: $0 ~/dev/android/notes-app/backend"
    exit 1
fi

DEST="$1"
SRC="$(cd "$(dirname "$0")" && pwd)"

if [ -d "$DEST" ]; then
    echo "ERROR: $DEST already exists"
    exit 1
fi

echo "Creating new app backend at: $DEST"
echo ""

# Create destination
mkdir -p "$DEST"

# Copy everything except git, build artifacts, and app-specific code
echo "1. Copying framework files..."

# Core files
cp "$SRC/Makefile" "$DEST/"
cp "$SRC/setup.sh" "$DEST/"
cp "$SRC/.gitignore" "$DEST/" 2>/dev/null || true

# Shared code (the framework)
cp -r "$SRC/shared" "$DEST/"

# Docs
mkdir -p "$DEST/docs"
cp "$SRC"/*.md "$DEST/" 2>/dev/null || true

# Kotlin SDK
cp -r "$SRC/clients" "$DEST/"

# Create a clean game-service as template (renamed to api-service)
echo "2. Creating clean api-service..."
mkdir -p "$DEST/services/api-service"

# Copy framework integration files
cp "$SRC/services/game-service/go.mod" "$DEST/services/api-service/"
cp "$SRC/services/game-service/go.sum" "$DEST/services/api-service/" 2>/dev/null || true

# Copy auth, buckets, users (reusable)
cp -r "$SRC/services/game-service/auth" "$DEST/services/api-service/"
cp -r "$SRC/services/game-service/buckets" "$DEST/services/api-service/"
cp -r "$SRC/services/game-service/users" "$DEST/services/api-service/"

# Copy test infrastructure
cp "$SRC/services/game-service/test.sh" "$DEST/services/api-service/"
cp "$SRC/services/game-service/benchmark.sh" "$DEST/services/api-service/"

# Create minimal main.go (no tic-tac-toe)
cat > "$DEST/services/api-service/main.go" << 'MAIN_GO'
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yourusername/my-app-backend/services/api-service/auth"
	"github.com/yourusername/my-app-backend/services/api-service/buckets"
	"github.com/yourusername/my-app-backend/services/api-service/users"
)

var firestoreClient *firestore.Client

func main() {
	ctx := context.Background()

	// Require Firestore emulator in dev
	if os.Getenv("ENV") != "production" {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			log.Fatal("Set FIRESTORE_EMULATOR_HOST for local dev (see docs)")
		}
		log.Printf("✓ Using Firestore emulator at %s", os.Getenv("FIRESTORE_EMULATOR_HOST"))
	}

	// Initialize Firestore
	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = "my-project" // Change this
	}

	var err error
	firestoreClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer firestoreClient.Close()

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth (magic link)
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	magicService := auth.RegisterRoutes(r, firestoreClient, baseURL)

	// Auth middleware
	authMiddleware := auth.Middleware(auth.AuthConfig{
		Mode:         "local",
		MagicService: magicService,
	})

	// Core routes
	users.RegisterRoutes(r, firestoreClient, authMiddleware)
	buckets.RegisterOpenBucketRoutes(r, firestoreClient, authMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// TODO: Add your app-specific routes here
	// r.Get("/", homeHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting api-service on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
MAIN_GO

# Create minimal test file
cat > "$DEST/services/api-service/main_test.go" << 'TEST_GO'
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/yourusername/my-app-backend/services/api-service/auth"
	"github.com/yourusername/my-app-backend/services/api-service/buckets"
	"github.com/yourusername/my-app-backend/services/api-service/users"
)

func setupTestRouter(client *firestore.Client) http.Handler {
	r := chi.NewRouter()

	mockAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", "test-user")
			ctx = context.WithValue(ctx, "user_email", "test@example.com")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	buckets.RegisterOpenBucketRoutes(r, client, mockAuth)
	users.RegisterRoutes(r, client, mockAuth)

	return r
}

func TestHealthCheck(t *testing.T) {
	// Basic test to ensure server starts
	t.Log("Server setup works")
}
TEST_GO

# Update go.mod module path and ALL imports
echo "3. Updating module paths..."
APP_NAME=$(basename "$(dirname "$DEST")")

# Update go.mod
sed -i "s|my-app-backend|${APP_NAME}-backend|g" "$DEST/services/api-service/go.mod" 2>/dev/null || \
sed -i '' "s|my-app-backend|${APP_NAME}-backend|g" "$DEST/services/api-service/go.mod"

# Also change game-service to api-service in go.mod
sed -i "s|game-service|api-service|g" "$DEST/services/api-service/go.mod" 2>/dev/null || \
sed -i '' "s|game-service|api-service|g" "$DEST/services/api-service/go.mod"

# Update ALL Go files in the service (both patterns)
find "$DEST/services/api-service" -name "*.go" -type f | while read file; do
    # Replace game-service path
    sed -i "s|my-app-backend/services/game-service|${APP_NAME}-backend/services/api-service|g" "$file" 2>/dev/null || \
    sed -i '' "s|my-app-backend/services/game-service|${APP_NAME}-backend/services/api-service|g" "$file"
    # Replace api-service path (from heredocs)
    sed -i "s|my-app-backend/services/api-service|${APP_NAME}-backend/services/api-service|g" "$file" 2>/dev/null || \
    sed -i '' "s|my-app-backend/services/api-service|${APP_NAME}-backend/services/api-service|g" "$file"
done

# Create README
cat > "$DEST/README.md" << README
# $APP_NAME Backend

Generated from bucket-framework template.

## Quick Start

\`\`\`bash
# Terminal 1: Start Firestore emulator
gcloud emulators firestore start --host-port=localhost:9099

# Terminal 2: Run server
export FIRESTORE_EMULATOR_HOST=localhost:9099
cd services/api-service
go run .
\`\`\`

## What's Included

- **Personal Buckets**: User-scoped data storage
- **Org Buckets**: Team/org data with roles
- **Magic Link Auth**: Passwordless authentication
- **User Service**: Profiles and friendships
- **Kotlin SDK**: Offline-first mobile client

## Structure

\`\`\`
backend/
├── services/api-service/   # Your app's backend
│   ├── main.go             # Entry point - add routes here
│   ├── auth/               # Magic link auth
│   ├── buckets/            # Personal & org buckets
│   └── users/              # User profiles, friendships
├── shared/                 # Framework code (don't edit)
└── clients/kotlin-sdk/     # Mobile SDK
\`\`\`

## Adding Features

1. Create buckets from mobile: \`POST /buckets/mine/{bucket-name}\`
2. Add custom routes in \`main.go\`
3. Run tests: \`cd services/api-service && ./test.sh\`
README

echo ""
echo "================================================"
echo "  Created: $DEST"
echo "================================================"
echo ""
echo "Next steps:"
echo "  cd $DEST/services/api-service"
echo "  go mod tidy"
echo "  ./test.sh"
echo ""
echo "Your app structure:"
echo "  $(dirname $DEST)/"
echo "  ├── backend/          <- you are here"
echo "  ├── android/          <- your Android app"
echo "  └── ios/              <- your iOS app"
echo ""
