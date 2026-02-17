package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const UserIDKey contextKey = "user_id"
const UserEmailKey contextKey = "user_email"

// CombinedMiddleware accepts local:* tokens (dev) OR session tokens (prod)
func CombinedMiddleware(authService *AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")

			if token == "" {
				respondUnauthorized(w, "missing token")
				return
			}

			// Local dev token: "local:<user_id>"
			if strings.HasPrefix(token, "local:") {
				userID := strings.TrimPrefix(token, "local:")
				if userID == "" {
					respondUnauthorized(w, "missing user_id in token")
					return
				}
				ctx := r.Context()
				ctx = context.WithValue(ctx, UserIDKey, userID)
				ctx = context.WithValue(ctx, UserEmailKey, userID+"@local.dev")
				ctx = context.WithValue(ctx, "user_id", userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Session token
			if authService != nil {
				session, err := authService.ValidateSession(r.Context(), token)
				if err == nil {
					ctx := r.Context()
					ctx = context.WithValue(ctx, UserIDKey, session.UserID)
					ctx = context.WithValue(ctx, UserEmailKey, session.Email)
					ctx = context.WithValue(ctx, "user_id", session.UserID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			respondUnauthorized(w, "invalid token")
		})
	}
}

func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	if id, ok := ctx.Value("user_id").(string); ok {
		return id
	}
	return ""
}

func respondUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthorized",
		"message": message,
	})
}
