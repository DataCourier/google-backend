package users

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Friendship represents a bidirectional friendship
// Stored with sorted user IDs to avoid duplicates
type Friendship struct {
	ID        string    `firestore:"id" json:"id"`
	UserA     string    `firestore:"user_a" json:"user_a"` // lexically smaller
	UserB     string    `firestore:"user_b" json:"user_b"` // lexically larger
	CreatedAt time.Time `firestore:"created_at" json:"created_at"`
}

// Invite represents a pending friend invite
type Invite struct {
	ID             string    `firestore:"id" json:"id"`
	SenderID       string    `firestore:"sender_id" json:"sender_id"`
	SenderName     string    `firestore:"sender_name" json:"sender_name"`
	RecipientEmail string    `firestore:"recipient_email" json:"recipient_email"`
	Token          string    `firestore:"token" json:"token"`
	Status         string    `firestore:"status" json:"status"` // pending, accepted, declined
	CreatedAt      time.Time `firestore:"created_at" json:"created_at"`
	AcceptedAt     time.Time `firestore:"accepted_at,omitempty" json:"accepted_at,omitempty"`
	AcceptedBy     string    `firestore:"accepted_by,omitempty" json:"accepted_by,omitempty"`
}

const (
	InviteStatusPending  = "pending"
	InviteStatusAccepted = "accepted"
	InviteStatusDeclined = "declined"
)

// FriendService handles friendships and invites
type FriendService struct {
	client      *firestore.Client
	userService *Service
}

// NewFriendService creates a new friend service
func NewFriendService(client *firestore.Client, userService *Service) *FriendService {
	return &FriendService{client: client, userService: userService}
}

// CreateInvite creates a friend invite sent to an email
func (s *FriendService) CreateInvite(ctx context.Context, senderID, senderName, recipientEmail string) (*Invite, error) {
	// Generate unique token
	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	invite := &Invite{
		ID:             generateID(),
		SenderID:       senderID,
		SenderName:     senderName,
		RecipientEmail: recipientEmail,
		Token:          token,
		Status:         InviteStatusPending,
		CreatedAt:      time.Now(),
	}

	_, err = s.client.Collection("friend-invites").Doc(invite.ID).Set(ctx, invite)
	if err != nil {
		return nil, err
	}

	return invite, nil
}

// GetInviteByToken retrieves an invite by its token
func (s *FriendService) GetInviteByToken(ctx context.Context, token string) (*Invite, error) {
	iter := s.client.Collection("friend-invites").
		Where("token", "==", token).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, errors.New("invite not found")
	}
	if err != nil {
		return nil, err
	}

	var invite Invite
	doc.DataTo(&invite)
	return &invite, nil
}

// AcceptInvite accepts an invite and creates friendship
func (s *FriendService) AcceptInvite(ctx context.Context, token, acceptorID string) (*Friendship, error) {
	invite, err := s.GetInviteByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if invite.Status != InviteStatusPending {
		return nil, errors.New("invite already " + invite.Status)
	}

	// Can't accept your own invite
	if invite.SenderID == acceptorID {
		return nil, errors.New("cannot accept your own invite")
	}

	// Create friendship
	friendship, err := s.createFriendship(ctx, invite.SenderID, acceptorID)
	if err != nil {
		return nil, err
	}

	// Mark invite as accepted
	invite.Status = InviteStatusAccepted
	invite.AcceptedAt = time.Now()
	invite.AcceptedBy = acceptorID
	_, err = s.client.Collection("friend-invites").Doc(invite.ID).Set(ctx, invite)
	if err != nil {
		return nil, err
	}

	return friendship, nil
}

// DeclineInvite declines an invite
func (s *FriendService) DeclineInvite(ctx context.Context, token, userID string) error {
	invite, err := s.GetInviteByToken(ctx, token)
	if err != nil {
		return err
	}

	if invite.Status != InviteStatusPending {
		return errors.New("invite already " + invite.Status)
	}

	invite.Status = InviteStatusDeclined
	_, err = s.client.Collection("friend-invites").Doc(invite.ID).Set(ctx, invite)
	return err
}

// createFriendship creates a friendship between two users
func (s *FriendService) createFriendship(ctx context.Context, userA, userB string) (*Friendship, error) {
	// Sort to ensure consistent ordering
	if userA > userB {
		userA, userB = userB, userA
	}

	// Check if already friends
	existing, _ := s.getFriendship(ctx, userA, userB)
	if existing != nil {
		return existing, nil // Already friends
	}

	friendship := &Friendship{
		ID:        userA + "-" + userB,
		UserA:     userA,
		UserB:     userB,
		CreatedAt: time.Now(),
	}

	_, err := s.client.Collection("friendships").Doc(friendship.ID).Set(ctx, friendship)
	if err != nil {
		return nil, err
	}

	return friendship, nil
}

// getFriendship gets existing friendship between two users
func (s *FriendService) getFriendship(ctx context.Context, userA, userB string) (*Friendship, error) {
	// Sort
	if userA > userB {
		userA, userB = userB, userA
	}

	doc, err := s.client.Collection("friendships").Doc(userA + "-" + userB).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var friendship Friendship
	doc.DataTo(&friendship)
	return &friendship, nil
}

// AreFriends checks if two users are friends
func (s *FriendService) AreFriends(ctx context.Context, userA, userB string) (bool, error) {
	friendship, err := s.getFriendship(ctx, userA, userB)
	if err != nil {
		return false, err
	}
	return friendship != nil, nil
}

// GetFriends returns all friends of a user
func (s *FriendService) GetFriends(ctx context.Context, userID string) ([]string, error) {
	var friendIDs []string

	// Query where user is userA
	iter := s.client.Collection("friendships").
		Where("user_a", "==", userID).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var f Friendship
		doc.DataTo(&f)
		friendIDs = append(friendIDs, f.UserB)
	}

	// Query where user is userB
	iter = s.client.Collection("friendships").
		Where("user_b", "==", userID).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var f Friendship
		doc.DataTo(&f)
		friendIDs = append(friendIDs, f.UserA)
	}

	return friendIDs, nil
}

// GetFriendProfiles returns profiles of all friends
func (s *FriendService) GetFriendProfiles(ctx context.Context, userID string) ([]*User, error) {
	friendIDs, err := s.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}

	var profiles []*User
	for _, id := range friendIDs {
		user, err := s.userService.Get(ctx, id)
		if err == nil && user != nil {
			profiles = append(profiles, user)
		}
	}

	return profiles, nil
}

// RemoveFriend removes a friendship
func (s *FriendService) RemoveFriend(ctx context.Context, userA, userB string) error {
	// Sort
	if userA > userB {
		userA, userB = userB, userA
	}

	_, err := s.client.Collection("friendships").Doc(userA + "-" + userB).Delete(ctx)
	return err
}

// GetPendingInvites returns invites sent by user
func (s *FriendService) GetPendingInvites(ctx context.Context, userID string) ([]*Invite, error) {
	iter := s.client.Collection("friend-invites").
		Where("sender_id", "==", userID).
		Where("status", "==", InviteStatusPending).
		Documents(ctx)

	var invites []*Invite
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var invite Invite
		doc.DataTo(&invite)
		invites = append(invites, &invite)
	}

	return invites, nil
}

// Helper functions

func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateID() string {
	token, _ := generateToken(16)
	return token
}
