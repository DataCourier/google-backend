package main

import (
	"log"
	"net/http"
	"os"

	"github.com/yourusername/my-app-backend/services/user-service/middleware"
	"github.com/yourusername/my-app-backend/services/user-service/response"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /health", healthHandler)

	// Hello world endpoint
	mux.HandleFunc("GET /", helloHandler)

	// User endpoints (placeholder)
	mux.HandleFunc("GET /users/me", getMeHandler)

	// Wrap with middleware
	handler := middleware.Logging(
		middleware.Recovery(
			middleware.CORS(mux),
		),
	)

	log.Printf("Starting user-service on port %s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response.Ok(w, "healthy", nil)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	response.Ok(w, "User Service API", map[string]string{
		"version": "0.1.0",
		"service": "user-service",
	})
}

func getMeHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify Firebase token
	// For now, return demo data
	response.Ok(w, "User profile", map[string]string{
		"id":    "demo-user-id",
		"email": "demo@example.com",
	})
}
