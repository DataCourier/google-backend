package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var (
	fsClient  *firestore.Client
	gcsBucket *storage.BucketHandle
	finnhub   *FinnhubClient
)

func main() {
	ctx := context.Background()

	// Firestore emulator check for local dev
	if os.Getenv("ENV") != "production" {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			log.Fatal("FIRESTORE_EMULATOR_HOST not set. Start the emulator first.")
		}
		log.Printf("Using Firestore emulator at %s", os.Getenv("FIRESTORE_EMULATOR_HOST"))
	}

	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = "michal-playground-2026"
	}

	// Firestore
	var err error
	fsClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fsClient.Close()

	// GCS
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCS client: %v", err)
	}
	defer storageClient.Close()

	bucketName := os.Getenv("GCS_BUCKET")
	if bucketName == "" {
		bucketName = "stock-prices-dev"
	}
	gcsBucket = storageClient.Bucket(bucketName)

	// Finnhub
	apiKey := os.Getenv("FINNHUB_API_KEY")
	if apiKey == "" {
		log.Fatal("FINNHUB_API_KEY not set")
	}
	finnhub = NewFinnhubClient(apiKey)

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/cron", cronHandler)
	r.Post("/heartbeat", heartbeatHandler)
	r.Post("/fetch", fetchHandler)
	r.Post("/fundamentals", fundamentalsHandler)
	r.Post("/universe", universeHandler)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Starting stock-prices service on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

type heartbeatRequest struct {
	Symbols []string `json:"symbols"`
}

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Symbols) == 0 {
		http.Error(w, "symbols required", http.StatusBadRequest)
		return
	}

	if len(req.Symbols) > 50 {
		http.Error(w, "max 50 symbols per heartbeat", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	for _, symbol := range req.Symbols {
		ref := fsClient.Collection("watched_symbols").Doc(symbol)
		_, err := ref.Set(ctx, map[string]interface{}{
			"symbol":           symbol,
			"subscribers_today": firestore.Increment(1),
		}, firestore.MergeAll)
		if err != nil {
			log.Printf("Failed to increment %s: %v", symbol, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func fetchHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := RunFetchCycle(ctx, fsClient, gcsBucket, finnhub); err != nil {
		log.Printf("Fetch cycle error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func cronHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	results := CronRouter(ctx, fsClient, gcsBucket, finnhub)

	// Log results
	response := map[string]string{}
	hasError := false
	for job, err := range results {
		if err != nil {
			log.Printf("CRON %s: ERROR %v", job, err)
			response[job] = "error: " + err.Error()
			hasError = true
		} else {
			response[job] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if hasError {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(response)
}

func universeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := RunUniverseUpdate(ctx, gcsBucket, finnhub); err != nil {
		log.Printf("Universe update error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func fundamentalsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := RunFundamentalsCycle(ctx, fsClient, gcsBucket, finnhub); err != nil {
		log.Printf("Fundamentals cycle error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
