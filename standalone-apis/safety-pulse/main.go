package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yourusername/safety-pulse/auth"
	"github.com/yourusername/safety-pulse/buckets"
)

var firestoreClient *firestore.Client

func main() {
	ctx := context.Background()

	// Require Firestore emulator in dev
	if os.Getenv("ENV") != "production" {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			log.Fatal("FIRESTORE_EMULATOR_HOST not set. Start the emulator: gcloud emulators firestore start --host-port=localhost:9090")
		}
		log.Printf("Using Firestore emulator at %s", os.Getenv("FIRESTORE_EMULATOR_HOST"))
	}

	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = "michal-playground-2026"
	}

	var err error
	firestoreClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer firestoreClient.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8083"
	}
	magicService := auth.RegisterRoutes(r, firestoreClient, baseURL)

	authMiddleware := auth.Middleware(auth.AuthConfig{
		Mode:         "local",
		MagicService: magicService,
	})

	// Org bucket routes (beacons, pings)
	buckets.RegisterOpenBucketRoutes(r, firestoreClient, authMiddleware)

	// Family routes (create, join, status)
	registerFamilyRoutes(r, firestoreClient, authMiddleware)

	// Beacon routes (ping, list pings, get, update, delete)
	registerBeaconRoutes(r, firestoreClient, authMiddleware)

	// Health check — verifies Firestore connectivity
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// Try a lightweight Firestore read to verify connectivity
		iter := firestoreClient.Collection("health-check").Limit(1).Documents(r.Context())
		iter.Stop()

		buckets.RespondJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	log.Printf("SafetyPulse starting on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
