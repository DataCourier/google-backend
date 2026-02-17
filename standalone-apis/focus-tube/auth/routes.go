package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes registers auth routes (unauthenticated)
func RegisterRoutes(r chi.Router, service *AuthService) {
	r.Post("/auth/request-code", requestCodeHandler(service))
	r.Post("/auth/verify-code", verifyCodeHandler(service))
	r.Post("/auth/logout", logoutHandler(service))
}

func requestCodeHandler(service *AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "email required"})
			return
		}

		ac, err := service.CreateAuthCode(r.Context(), req.Email)
		if err != nil {
			log.Printf("CreateAuthCode error: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create code"})
			return
		}

		// Return code in response (personal app, no email needed)
		respondJSON(w, http.StatusOK, map[string]string{
			"code": ac.Code,
		})
	}
}

func verifyCodeHandler(service *AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" || req.Code == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "email and code required"})
			return
		}

		if err := service.VerifyAuthCode(r.Context(), req.Email, req.Code); err != nil {
			respondJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}

		userID, err := service.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
			return
		}

		session, err := service.CreateSession(r.Context(), userID, req.Email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"token":   session.Token,
			"user_id": session.UserID,
			"email":   session.Email,
		})
	}
}

func logoutHandler(service *AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")

		if token == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "missing token"})
			return
		}

		service.RevokeSession(r.Context(), token)
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
