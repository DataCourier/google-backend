package users

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const CollectionName = "users"

// User represents a user profile
type User struct {
	ID        string    `firestore:"id" json:"id"`
	Email     string    `firestore:"email" json:"email"`
	Name      string    `firestore:"name" json:"name"`
	PhotoURL  string    `firestore:"photo_url,omitempty" json:"photo_url,omitempty"`
	CreatedAt time.Time `firestore:"created_at" json:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at" json:"updated_at"`

	// Optional profile fields
	Bio      string `firestore:"bio,omitempty" json:"bio,omitempty"`
	Location string `firestore:"location,omitempty" json:"location,omitempty"`
}

// Service handles user operations
type Service struct {
	client *firestore.Client
}

// NewService creates a new user service
func NewService(client *firestore.Client) *Service {
	return &Service{client: client}
}

// GetOrCreate gets existing user or creates new one
// Called on every authenticated request to ensure user exists
func (s *Service) GetOrCreate(ctx context.Context, id, email, name string) (*User, error) {
	doc, err := s.client.Collection(CollectionName).Doc(id).Get(ctx)

	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Create new user
			user := &User{
				ID:        id,
				Email:     email,
				Name:      name,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			_, err = s.client.Collection(CollectionName).Doc(id).Set(ctx, user)
			if err != nil {
				return nil, err
			}

			return user, nil
		}
		return nil, err
	}

	// User exists
	var user User
	doc.DataTo(&user)
	return &user, nil
}

// Get retrieves a user by ID
func (s *Service) Get(ctx context.Context, id string) (*User, error) {
	doc, err := s.client.Collection(CollectionName).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // Not found, return nil
		}
		return nil, err
	}

	var user User
	doc.DataTo(&user)
	return &user, nil
}

// Update updates user profile fields
func (s *Service) Update(ctx context.Context, id string, updates map[string]interface{}) (*User, error) {
	updates["updated_at"] = time.Now()

	// Prevent changing ID
	delete(updates, "id")
	delete(updates, "created_at")

	_, err := s.client.Collection(CollectionName).Doc(id).Set(ctx, updates, firestore.MergeAll)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a user (careful!)
func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.client.Collection(CollectionName).Doc(id).Delete(ctx)
	return err
}
