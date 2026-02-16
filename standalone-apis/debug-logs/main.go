package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/api/iterator"
)

var (
	fsClient *firestore.Client
	apiKey   string
)

func main() {
	ctx := context.Background()

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

	var err error
	fsClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer fsClient.Close()

	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY not set")
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth)
		r.Post("/log", postLogHandler)
		r.Get("/log/{app}", getLogHandler)
		r.Get("/logs", listLogsHandler)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Starting debug-logs service on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

func apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// POST /log
// Body: {"app": "ticker-widget", "logs": <anything>}
// Stores the latest debug dump for that app, overwriting previous.
func postLogHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App  string          `json:"app"`
		Logs json.RawMessage `json:"logs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.App == "" {
		http.Error(w, "app required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	_, err := fsClient.Collection("debug_logs").Doc(req.App).Set(ctx, map[string]interface{}{
		"app":        req.App,
		"logs":       string(req.Logs),
		"updated_at": time.Now().UnixMilli(),
	})
	if err != nil {
		log.Printf("Debug write error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /log/{app}
// Returns the latest debug dump for that app.
func getLogHandler(w http.ResponseWriter, r *http.Request) {
	app := chi.URLParam(r, "app")
	ctx := r.Context()

	doc, err := fsClient.Collection("debug_logs").Doc(app).Get(ctx)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data := doc.Data()

	// Parse logs back to JSON object if possible
	if logsStr, ok := data["logs"].(string); ok {
		var parsed json.RawMessage
		if json.Unmarshal([]byte(logsStr), &parsed) == nil {
			data["logs"] = parsed
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GET /logs
// Lists all apps that have debug logs.
func listLogsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	iter := fsClient.Collection("debug_logs").Documents(ctx)

	var apps []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := doc.Data()
		// Just return app name and timestamp, not the full logs
		apps = append(apps, map[string]interface{}{
			"app":        data["app"],
			"updated_at": data["updated_at"],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}
