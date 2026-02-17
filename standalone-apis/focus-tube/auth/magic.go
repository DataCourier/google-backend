package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	AuthCodeCollection = "auth-codes"
	AuthCodeExpiry     = 5 * time.Minute
	SessionCollection  = "sessions"
	SessionExpiry      = 30 * 24 * time.Hour
)

type AuthCode struct {
	ID        string    `firestore:"id"`
	Email     string    `firestore:"email"`
	Code      string    `firestore:"code"`
	ExpiresAt time.Time `firestore:"expires_at"`
	Used      bool      `firestore:"used"`
	CreatedAt time.Time `firestore:"created_at"`
}

type Session struct {
	ID        string    `firestore:"id"`
	UserID    string    `firestore:"user_id"`
	Email     string    `firestore:"email"`
	Token     string    `firestore:"token"`
	ExpiresAt time.Time `firestore:"expires_at"`
	CreatedAt time.Time `firestore:"created_at"`
}

type AuthService struct {
	client *firestore.Client
}

func NewAuthService(client *firestore.Client) *AuthService {
	return &AuthService{client: client}
}

// CreateAuthCode generates a 6-digit code for the given email
func (s *AuthService) CreateAuthCode(ctx context.Context, email string) (*AuthCode, error) {
	code, err := generate6DigitCode()
	if err != nil {
		return nil, err
	}

	ac := &AuthCode{
		ID:        generateID(),
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(AuthCodeExpiry),
		Used:      false,
		CreatedAt: time.Now(),
	}

	_, err = s.client.Collection(AuthCodeCollection).Doc(ac.ID).Set(ctx, ac)
	if err != nil {
		return nil, err
	}

	return ac, nil
}

// VerifyAuthCode validates a code for an email
func (s *AuthService) VerifyAuthCode(ctx context.Context, email, code string) error {
	iter := s.client.Collection(AuthCodeCollection).
		Where("email", "==", email).
		Where("code", "==", code).
		Where("used", "==", false).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return errors.New("invalid or expired code")
	}
	if err != nil {
		return err
	}

	var ac AuthCode
	doc.DataTo(&ac)

	if time.Now().After(ac.ExpiresAt) {
		return errors.New("code expired")
	}

	// Mark as used
	_, err = s.client.Collection(AuthCodeCollection).Doc(ac.ID).Update(ctx, []firestore.Update{
		{Path: "used", Value: true},
	})
	return err
}

// CreateSession creates a new session for user
func (s *AuthService) CreateSession(ctx context.Context, userID, email string) (*Session, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:        generateID(),
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
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*Session, error) {
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

// RevokeSession revokes a session by token
func (s *AuthService) RevokeSession(ctx context.Context, token string) error {
	iter := s.client.Collection(SessionCollection).
		Where("token", "==", token).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil
	}
	if err != nil {
		return err
	}

	_, err = doc.Ref.Delete(ctx)
	return err
}

// GetUserByEmail finds or creates user by email, returns user ID
func (s *AuthService) GetUserByEmail(ctx context.Context, email string) (string, error) {
	iter := s.client.Collection("users").
		Where("email", "==", email).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		// Create new user
		userID := generateID()
		user := map[string]interface{}{
			"id":         userID,
			"email":      email,
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

	data := doc.Data()
	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return doc.Ref.ID, nil
}

// GetSession gets session by ID
func (s *AuthService) GetSession(ctx context.Context, sessionID string) (*Session, error) {
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

func generate6DigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateID() string {
	t, _ := generateSecureToken(8)
	return t
}
