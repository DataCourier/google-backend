package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	MagicLinkCollection = "magic-links"
	MagicLinkExpiry     = 15 * time.Minute
)

// MagicLink represents a pending magic link
type MagicLink struct {
	ID        string    `firestore:"id"`
	Email     string    `firestore:"email"`
	Token     string    `firestore:"token"`
	ExpiresAt time.Time `firestore:"expires_at"`
	Used      bool      `firestore:"used"`
	CreatedAt time.Time `firestore:"created_at"`
}

// MagicLinkService handles magic link auth
type MagicLinkService struct {
	client  *firestore.Client
	baseURL string
}

// NewMagicLinkService creates new service
func NewMagicLinkService(client *firestore.Client, baseURL string) *MagicLinkService {
	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &MagicLinkService{client: client, baseURL: baseURL}
}

// CreateMagicLink creates a magic link for email
func (s *MagicLinkService) CreateMagicLink(ctx context.Context, email string) (*MagicLink, string, error) {
	// Generate token
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, "", err
	}

	ml := &MagicLink{
		ID:        generateSecureToken16(),
		Email:     email,
		Token:     token,
		ExpiresAt: time.Now().Add(MagicLinkExpiry),
		Used:      false,
		CreatedAt: time.Now(),
	}

	_, err = s.client.Collection(MagicLinkCollection).Doc(ml.ID).Set(ctx, ml)
	if err != nil {
		return nil, "", err
	}

	// Build magic link URL
	magicURL := s.baseURL + "/auth/verify?token=" + token

	// In dev mode, log the link
	if os.Getenv("ENV") != "production" {
		log.Printf("🔗 Magic link for %s: %s", email, magicURL)
	}

	return ml, magicURL, nil
}

// VerifyMagicLink verifies token and returns email
func (s *MagicLinkService) VerifyMagicLink(ctx context.Context, token string) (string, error) {
	// Find magic link by token
	iter := s.client.Collection(MagicLinkCollection).
		Where("token", "==", token).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return "", errors.New("invalid or expired link")
	}
	if err != nil {
		return "", err
	}

	var ml MagicLink
	doc.DataTo(&ml)

	// Check if already used
	if ml.Used {
		return "", errors.New("link already used")
	}

	// Check expiry
	if time.Now().After(ml.ExpiresAt) {
		return "", errors.New("link expired")
	}

	// Mark as used
	_, err = s.client.Collection(MagicLinkCollection).Doc(ml.ID).Update(ctx, []firestore.Update{
		{Path: "used", Value: true},
	})
	if err != nil {
		return "", err
	}

	return ml.Email, nil
}

// CleanupExpired removes old magic links (call periodically)
func (s *MagicLinkService) CleanupExpired(ctx context.Context) error {
	cutoff := time.Now().Add(-24 * time.Hour)

	iter := s.client.Collection(MagicLinkCollection).
		Where("created_at", "<", cutoff).
		Documents(ctx)

	batch := s.client.Batch()
	count := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		batch.Delete(doc.Ref)
		count++
	}

	if count > 0 {
		_, err := batch.Commit(ctx)
		return err
	}

	return nil
}

// Session represents an authenticated session
type Session struct {
	ID        string    `firestore:"id"`
	UserID    string    `firestore:"user_id"`
	Email     string    `firestore:"email"`
	Token     string    `firestore:"token"` // Session token (not magic link token)
	ExpiresAt time.Time `firestore:"expires_at"`
	CreatedAt time.Time `firestore:"created_at"`
}

const (
	SessionCollection = "sessions"
	SessionExpiry     = 30 * 24 * time.Hour // 30 days
)

// CreateSession creates a new session for user
func (s *MagicLinkService) CreateSession(ctx context.Context, userID, email string) (*Session, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:        generateSecureToken16(),
		UserID:    userID,
		Email:     email,
		Token:     token,
		ExpiresAt: time.Now().Add(SessionExpiry),
		CreatedAt: time.Now(),
	}

	_, err = s.client.Collection(SessionCollection).Doc(session.ID).Set(ctx, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// ValidateSession validates a session token
func (s *MagicLinkService) ValidateSession(ctx context.Context, token string) (*Session, error) {
	iter := s.client.Collection(SessionCollection).
		Where("token", "==", token).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, errors.New("invalid session")
	}
	if err != nil {
		return nil, err
	}

	var session Session
	doc.DataTo(&session)

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}

	return &session, nil
}

// RevokeSession revokes a session
func (s *MagicLinkService) RevokeSession(ctx context.Context, token string) error {
	iter := s.client.Collection(SessionCollection).
		Where("token", "==", token).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil // Already gone
	}
	if err != nil {
		return err
	}

	_, err = doc.Ref.Delete(ctx)
	return err
}

// RevokeAllSessions revokes all sessions for a user
func (s *MagicLinkService) RevokeAllSessions(ctx context.Context, userID string) error {
	iter := s.client.Collection(SessionCollection).
		Where("user_id", "==", userID).
		Documents(ctx)

	batch := s.client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		batch.Delete(doc.Ref)
	}

	_, err := batch.Commit(ctx)
	return err
}

// GetUserByEmail finds or creates user by email, returns user ID
func (s *MagicLinkService) GetUserByEmail(ctx context.Context, email string) (string, error) {
	// Look up user by email
	iter := s.client.Collection("users").
		Where("email", "==", email).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		// Create new user
		userID := generateSecureToken16()
		user := map[string]interface{}{
			"id":         userID,
			"email":      email,
			"name":       emailToName(email),
			"created_at": time.Now(),
			"updated_at": time.Now(),
		}
		_, err = s.client.Collection("users").Doc(userID).Set(ctx, user)
		if err != nil {
			return "", err
		}
		return userID, nil
	}
	if err != nil {
		return "", err
	}

	// Return existing user ID
	data := doc.Data()
	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return doc.Ref.ID, nil
}

// Helper: extract name from email
func emailToName(email string) string {
	for i, c := range email {
		if c == '@' {
			return email[:i]
		}
	}
	return email
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateSecureToken16() string {
	t, _ := generateSecureToken(8)
	return t
}

// GetSession gets session by ID (for cleanup)
func (s *MagicLinkService) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	doc, err := s.client.Collection(SessionCollection).Doc(sessionID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var session Session
	doc.DataTo(&session)
	return &session, nil
}
