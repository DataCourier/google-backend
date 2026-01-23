package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yourusername/my-app-backend/services/game-service/models"
)

var (
	firestoreClient *firestore.Client
	templates       *template.Template
)

func main() {
	ctx := context.Background()

	// Initialize Firestore (uses GOOGLE_APPLICATION_CREDENTIALS env var)
	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = "michal-playground-2026" // fallback for local dev
	}

	var err error
	firestoreClient, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer firestoreClient.Close()

	// Parse templates - each template file is parsed independently
	// and can reference layout.html
	templates = template.Must(template.ParseGlob("views/*.html"))

	// Setup routes with chi router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Specific routes first
	r.Post("/games/{gameId}/move", makeMoveHandler)
	r.Get("/games/{gameId}", viewGameHandler)
	r.Post("/games/create", createGameHandler)

	// Catch-all route last
	r.Get("/", homeHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting game-service on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ homeHandler: %s %s", r.Method, r.URL.Path)

	if err := templates.ExecuteTemplate(w, "home.html", nil); err != nil {
		log.Printf("✗ homeHandler: template error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("✓ homeHandler: rendered home.html")
}

func createGameHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("→ createGameHandler: %s %s", r.Method, r.URL.Path)
	ctx := r.Context()

	game, err := models.CreateGame(ctx, firestoreClient)
	if err != nil {
		log.Printf("✗ createGameHandler: failed to create game: %v", err)
		http.Error(w, "Failed to create game", http.StatusInternalServerError)
		return
	}

	// Redirect player X to their game URL
	playerXURL := fmt.Sprintf("/games/%s?token=%s", game.ID, game.PlayerXToken)
	log.Printf("✓ createGameHandler: created game %s, redirecting to %s", game.ID, playerXURL)
	http.Redirect(w, r, playerXURL, http.StatusSeeOther)
}

func viewGameHandler(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	log.Printf("→ viewGameHandler: %s %s (gameID=%s)", r.Method, r.URL.Path, gameID)

	ctx := r.Context()
	token := r.URL.Query().Get("token")

	if token == "" {
		log.Printf("✗ viewGameHandler: missing token")
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}

	game, err := models.GetGame(ctx, firestoreClient, gameID)
	if err != nil {
		log.Printf("✗ viewGameHandler: game not found: %v", err)
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	// Validate token and determine player
	player, err := game.ValidateToken(token)
	if err != nil {
		log.Printf("✗ viewGameHandler: invalid token")
		http.Error(w, "Invalid token", http.StatusForbidden)
		return
	}

	log.Printf("  viewGameHandler: player=%s, turn=%s, winner=%s", player, game.CurrentTurn, game.Winner)

	// Build template data
	data := map[string]interface{}{
		"GameID":      game.ID,
		"Token":       token,
		"Player":      player,
		"Board":       game.Board,
		"CurrentTurn": game.CurrentTurn,
		"Winner":      game.Winner,
	}

	// Status message
	if game.Winner != "" {
		if game.Winner == "draw" {
			data["Status"] = "Game ended in a draw!"
		} else {
			data["Status"] = fmt.Sprintf("Player %s wins!", game.Winner)
		}
	} else if game.CurrentTurn == player {
		data["Status"] = fmt.Sprintf("You are %s - Your turn!", player)
	} else {
		data["Status"] = fmt.Sprintf("You are %s - Waiting for opponent...", player)
	}

	// Show share link only for player X and only at the start
	if player == "X" && game.Winner == "" {
		// Count moves to see if game just started
		moveCount := 0
		for _, cell := range game.Board {
			if cell != "" {
				moveCount++
			}
		}

		if moveCount == 0 {
			// Generate opponent URL
			baseURL := getBaseURL(r)
			opponentURL := fmt.Sprintf("%s/games/%s?token=%s", baseURL, game.ID, game.PlayerOToken)
			data["ShowShareLink"] = true
			data["OpponentURL"] = opponentURL
		}
	}

	log.Printf("  viewGameHandler: rendering template 'game.html' with data keys: %v", getKeys(data))
	if err := templates.ExecuteTemplate(w, "game.html", data); err != nil {
		log.Printf("✗ viewGameHandler: template error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("✓ viewGameHandler: rendered game.html")
}

func makeMoveHandler(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	log.Printf("→ makeMoveHandler: %s %s (gameID=%s)", r.Method, r.URL.Path, gameID)

	ctx := r.Context()
	token := r.URL.Query().Get("token")

	if token == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "Missing token",
		})
		return
	}

	// Parse request body
	var req struct {
		Position int `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "Invalid JSON",
		})
		return
	}

	// Get game
	game, err := models.GetGame(ctx, firestoreClient, gameID)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error":   "not_found",
			"message": "Game not found",
		})
		return
	}

	// Validate token
	player, err := game.ValidateToken(token)
	if err != nil {
		respondJSON(w, http.StatusForbidden, map[string]string{
			"error":   "forbidden",
			"message": "Invalid token",
		})
		return
	}

	// Make move
	log.Printf("  makeMoveHandler: player=%s attempting move at position %d", player, req.Position)
	if err := game.MakeMove(ctx, firestoreClient, player, req.Position); err != nil {
		log.Printf("✗ makeMoveHandler: invalid move: %v", err)
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "invalid_move",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✓ makeMoveHandler: move successful, winner=%s", game.Winner)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Move made successfully",
		"board":   game.Board,
		"winner":  game.Winner,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func getBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
