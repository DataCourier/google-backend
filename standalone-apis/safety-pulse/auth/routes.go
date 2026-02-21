package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
)

// RegisterRoutes registers auth routes
func RegisterRoutes(r chi.Router, client *firestore.Client, baseURL string) *MagicLinkService {
	magicService := NewMagicLinkService(client, baseURL)

	r.Route("/auth", func(r chi.Router) {
		// POST /auth/magic-link - Request magic link
		r.Post("/magic-link", requestMagicLinkHandler(magicService))

		// GET /auth/verify - Verify magic link and get session
		r.Get("/verify", verifyMagicLinkHandler(magicService))

		// POST /auth/logout - Logout (revoke session)
		r.Post("/logout", logoutHandler(magicService))
	})

	return magicService
}

func requestMagicLinkHandler(service *MagicLinkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		email := strings.TrimSpace(strings.ToLower(req.Email))
		if email == "" || !strings.Contains(email, "@") {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "valid email required"})
			return
		}

		_, magicURL, err := service.CreateMagicLink(ctx, email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// In production, send email here
		// For now, we just return success (link is logged in dev)

		response := map[string]interface{}{
			"message": "check your email for the login link",
		}

		// In dev mode, also return the link for easy testing
		if r.Header.Get("X-Dev-Mode") == "true" {
			response["magic_url"] = magicURL
		}

		respondJSON(w, http.StatusOK, response)
	}
}

func verifyMagicLinkHandler(service *MagicLinkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := r.URL.Query().Get("token")

		if token == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
			return
		}

		// Verify magic link
		email, err := service.VerifyMagicLink(ctx, token)
		if err != nil {
			respondJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}

		// Get or create user
		userID, err := service.GetUserByEmail(ctx, email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Create session
		session, err := service.CreateSession(ctx, userID, email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message":    "logged in",
			"token":      session.Token,
			"user_id":    userID,
			"email":      email,
			"expires_at": session.ExpiresAt,
		})
	}
}

func logoutHandler(service *MagicLinkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")

		if token == "" || token == authHeader {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
			return
		}

		err := service.RevokeSession(ctx, token)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
