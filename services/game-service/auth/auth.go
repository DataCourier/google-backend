package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type contextKey string

const UserIDKey contextKey = "user_id"
const UserEmailKey contextKey = "user_email"
const UserNameKey contextKey = "user_name"

// AuthConfig determines auth mode
type AuthConfig struct {
	Mode string // "local" or "firebase"
}

// Middleware returns auth middleware based on config
func Middleware(cfg AuthConfig) func(http.Handler) http.Handler {
	if cfg.Mode == "firebase" {
		return firebaseAuthMiddleware()
	}
	return localAuthMiddleware()
}

// localAuthMiddleware - for development without Firebase
// Accepts tokens in format: "local:<user_id>" or "local:<user_id>:<email>:<name>"
// Example: Authorization: local:user-123
// Example: Authorization: local:user-123:alice@example.com:Alice
func localAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")

			// Strip "Bearer " prefix if present
			token = strings.TrimPrefix(token, "Bearer ")

			if token == "" {
				respondUnauthorized(w, "missing token")
				return
			}

			// Parse local token: "local:<user_id>" or "local:<user_id>:<email>:<name>"
			if !strings.HasPrefix(token, "local:") {
				respondUnauthorized(w, "invalid token format (use 'local:<user_id>')")
				return
			}

			parts := strings.Split(strings.TrimPrefix(token, "local:"), ":")
			if len(parts) < 1 || parts[0] == "" {
				respondUnauthorized(w, "missing user_id in token")
				return
			}

			userID := parts[0]
			email := userID + "@local.dev"
			name := userID

			if len(parts) >= 2 {
				email = parts[1]
			}
			if len(parts) >= 3 {
				name = parts[2]
			}

			// Set user info in context
			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, userID)
			ctx = context.WithValue(ctx, UserEmailKey, email)
			ctx = context.WithValue(ctx, UserNameKey, name)
			// Also set as string key for backward compatibility
			ctx = context.WithValue(ctx, "user_id", userID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// firebaseAuthMiddleware - for production with Firebase Auth
// Validates Firebase ID tokens
func firebaseAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement Firebase token validation
			// For now, fall back to local mode with a warning
			if os.Getenv("FIREBASE_AUTH_EMULATOR_HOST") != "" {
				// Firebase emulator mode - could validate against emulator
				// For now, treat like local
			}

			respondUnauthorized(w, "firebase auth not yet implemented")
		})
	}
}

// Helper to get user ID from context
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	// Fallback for string key
	if id, ok := ctx.Value("user_id").(string); ok {
		return id
	}
	return ""
}

// Helper to get user email from context
func GetUserEmail(ctx context.Context) string {
	if email, ok := ctx.Value(UserEmailKey).(string); ok {
		return email
	}
	// Fallback for string key (tests)
	if email, ok := ctx.Value("user_email").(string); ok {
		return email
	}
	return ""
}

// Helper to get user name from context
func GetUserName(ctx context.Context) string {
	if name, ok := ctx.Value(UserNameKey).(string); ok {
		return name
	}
	// Fallback for string key (tests)
	if name, ok := ctx.Value("user_name").(string); ok {
		return name
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
