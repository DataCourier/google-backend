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
	Mode         string            // "local", "session", or "firebase"
	MagicService *MagicLinkService // Required for session mode
}

// Middleware returns auth middleware based on config
func Middleware(cfg AuthConfig) func(http.Handler) http.Handler {
	switch cfg.Mode {
	case "firebase":
		return firebaseAuthMiddleware()
	case "session":
		return sessionAuthMiddleware(cfg.MagicService)
	default:
		// "local" or fallback - accepts both local tokens and session tokens
		return combinedAuthMiddleware(cfg.MagicService)
	}
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

// sessionAuthMiddleware - validates session tokens from magic link auth
func sessionAuthMiddleware(magicService *MagicLinkService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")

			if token == "" {
				respondUnauthorized(w, "missing token")
				return
			}

			if magicService == nil {
				respondUnauthorized(w, "auth not configured")
				return
			}

			session, err := magicService.ValidateSession(r.Context(), token)
			if err != nil {
				respondUnauthorized(w, err.Error())
				return
			}

			// Set user info in context
			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, session.UserID)
			ctx = context.WithValue(ctx, UserEmailKey, session.Email)
			ctx = context.WithValue(ctx, "user_id", session.UserID)
			ctx = context.WithValue(ctx, "user_email", session.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// combinedAuthMiddleware - accepts local tokens OR session tokens
// For development: use local:user-id format
// For production-like: use session tokens from magic link
func combinedAuthMiddleware(magicService *MagicLinkService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")

			if token == "" {
				respondUnauthorized(w, "missing token")
				return
			}

			// Check if it's a local dev token
			if strings.HasPrefix(token, "local:") {
				// Use local auth logic
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

				ctx := r.Context()
				ctx = context.WithValue(ctx, UserIDKey, userID)
				ctx = context.WithValue(ctx, UserEmailKey, email)
				ctx = context.WithValue(ctx, UserNameKey, name)
				ctx = context.WithValue(ctx, "user_id", userID)
				ctx = context.WithValue(ctx, "user_email", email)
				ctx = context.WithValue(ctx, "user_name", name)

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try session token
			if magicService != nil {
				session, err := magicService.ValidateSession(r.Context(), token)
				if err == nil {
					ctx := r.Context()
					ctx = context.WithValue(ctx, UserIDKey, session.UserID)
					ctx = context.WithValue(ctx, UserEmailKey, session.Email)
					ctx = context.WithValue(ctx, "user_id", session.UserID)
					ctx = context.WithValue(ctx, "user_email", session.Email)

					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			respondUnauthorized(w, "invalid token")
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

// AdminMiddleware validates admin API key from ADMIN_API_KEY env var
// Header: X-Admin-Key: <key>
func AdminMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adminKey := os.Getenv("ADMIN_API_KEY")
			if adminKey == "" {
				// No admin key configured - reject all admin requests
				respondUnauthorized(w, "admin access not configured")
				return
			}

			providedKey := r.Header.Get("X-Admin-Key")
			if providedKey == "" {
				respondUnauthorized(w, "missing X-Admin-Key header")
				return
			}

			if providedKey != adminKey {
				respondUnauthorized(w, "invalid admin key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
