package auth

import (
	"context"
	"net/http"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

// FirebaseAuth wraps Firebase authentication
type FirebaseAuth struct {
	client *auth.Client
}

// NewFirebaseAuth initializes Firebase auth client
func NewFirebaseAuth(ctx context.Context) (*FirebaseAuth, error) {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}

	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}

	return &FirebaseAuth{client: client}, nil
}

// VerifyToken verifies a Firebase ID token from Authorization header
func (f *FirebaseAuth) VerifyToken(ctx context.Context, r *http.Request) (*auth.Token, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, ErrNoAuthHeader
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, ErrInvalidAuthHeader
	}

	token := parts[1]
	return f.client.VerifyIDToken(ctx, token)
}

// Errors
var (
	ErrNoAuthHeader      = &AuthError{Code: "no_auth_header", Message: "Authorization header missing"}
	ErrInvalidAuthHeader = &AuthError{Code: "invalid_auth_header", Message: "Invalid Authorization header format"}
)

type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}
