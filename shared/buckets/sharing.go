package buckets

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

const (
	SharingAccessRead  = "read"
	SharingAccessWrite = "write"
)

// Sharing represents a share of a personal bucket item with another user
type Sharing struct {
	ID         string    `firestore:"id" json:"id"`
	Bucket     string    `firestore:"bucket" json:"bucket"`           // e.g., "notes"
	ItemID     string    `firestore:"item_id" json:"item_id"`         // ID in personal bucket
	Owner      string    `firestore:"owner" json:"owner"`             // who owns the item
	SharedWith string    `firestore:"shared_with" json:"shared_with"` // who it's shared with
	Access     string    `firestore:"access" json:"access"`           // read or write
	CreatedAt  time.Time `firestore:"created_at" json:"created_at"`
}

type SharingService struct {
	client         *firestore.Client
	personalBucket *PersonalBucketImpl
}

func NewSharingService(client *firestore.Client, personalBucket *PersonalBucketImpl) *SharingService {
	return &SharingService{client: client, personalBucket: personalBucket}
}

// Share an item from your personal bucket with another user
func (s *SharingService) Share(ctx context.Context, bucket, itemID, targetUserID, access string) (*Sharing, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	// Verify you own the item
	item, err := s.personalBucket.Get(ctx, bucket, itemID)
	if err != nil {
		return nil, err
	}
	if item["user_id"] != userID {
		return nil, errors.New("forbidden: not your item")
	}

	// Can't share with yourself
	if targetUserID == userID {
		return nil, errors.New("bad request: cannot share with yourself")
	}

	// Check if already shared
	existing, _ := s.getSharing(ctx, bucket, itemID, targetUserID)
	if existing != nil {
		// Update access level
		existing.Access = access
		s.client.Collection("sharings").Doc(existing.ID).Set(ctx, existing)
		return existing, nil
	}

	sharing := &Sharing{
		ID:         uuid.New().String(),
		Bucket:     bucket,
		ItemID:     itemID,
		Owner:      userID,
		SharedWith: targetUserID,
		Access:     access,
		CreatedAt:  time.Now(),
	}

	_, err = s.client.Collection("sharings").Doc(sharing.ID).Set(ctx, sharing)
	if err != nil {
		return nil, err
	}

	return sharing, nil
}

// Unshare removes sharing
func (s *SharingService) Unshare(ctx context.Context, bucket, itemID, targetUserID string) error {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return errors.New("unauthorized")
	}

	sharing, err := s.getSharing(ctx, bucket, itemID, targetUserID)
	if err != nil {
		return err
	}
	if sharing == nil {
		return errors.New("not found")
	}

	// Only owner can unshare
	if sharing.Owner != userID {
		return errors.New("forbidden")
	}

	_, err = s.client.Collection("sharings").Doc(sharing.ID).Delete(ctx)
	return err
}

// SharedWithMe returns items others have shared with me
func (s *SharingService) SharedWithMe(ctx context.Context) ([]Sharing, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	iter := s.client.Collection("sharings").Where("shared_with", "==", userID).Documents(ctx)
	var sharings []Sharing

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var sharing Sharing
		doc.DataTo(&sharing)
		sharings = append(sharings, sharing)
	}

	return sharings, nil
}

// SharedByMe returns items I've shared with others
func (s *SharingService) SharedByMe(ctx context.Context) ([]Sharing, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	iter := s.client.Collection("sharings").Where("owner", "==", userID).Documents(ctx)
	var sharings []Sharing

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var sharing Sharing
		doc.DataTo(&sharing)
		sharings = append(sharings, sharing)
	}

	return sharings, nil
}

// GetSharedItem retrieves an item that was shared with you
func (s *SharingService) GetSharedItem(ctx context.Context, bucket, itemID string) (map[string]interface{}, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	// Check if shared with me
	sharing, err := s.getSharingForRecipient(ctx, bucket, itemID, userID)
	if err != nil {
		return nil, err
	}
	if sharing == nil {
		return nil, errors.New("forbidden: not shared with you")
	}

	// Get from owner's personal bucket (bypass user check)
	return s.getItemAsOwner(ctx, bucket, itemID, sharing.Owner)
}

// UpdateSharedItem updates an item that was shared with you (if you have write access)
func (s *SharingService) UpdateSharedItem(ctx context.Context, bucket, itemID string, data map[string]interface{}) error {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return errors.New("unauthorized")
	}

	sharing, err := s.getSharingForRecipient(ctx, bucket, itemID, userID)
	if err != nil {
		return err
	}
	if sharing == nil {
		return errors.New("forbidden: not shared with you")
	}
	if sharing.Access != SharingAccessWrite {
		return errors.New("forbidden: read-only access")
	}

	// Update in owner's personal bucket
	return s.updateItemAsOwner(ctx, bucket, itemID, sharing.Owner, data)
}

// Helper: get sharing record for a specific recipient
func (s *SharingService) getSharingForRecipient(ctx context.Context, bucket, itemID, recipientID string) (*Sharing, error) {
	iter := s.client.Collection("sharings").
		Where("bucket", "==", bucket).
		Where("item_id", "==", itemID).
		Where("shared_with", "==", recipientID).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var sharing Sharing
	doc.DataTo(&sharing)
	return &sharing, nil
}

// Helper: get sharing record
func (s *SharingService) getSharing(ctx context.Context, bucket, itemID, sharedWith string) (*Sharing, error) {
	return s.getSharingForRecipient(ctx, bucket, itemID, sharedWith)
}

// Helper: get item bypassing user check (for shared access)
func (s *SharingService) getItemAsOwner(ctx context.Context, bucket, itemID, ownerID string) (map[string]interface{}, error) {
	collection := "personal-" + bucket
	doc, err := s.client.Collection(collection).Doc(itemID).Get(ctx)
	if err != nil {
		return nil, err
	}
	data := doc.Data()
	if data["user_id"] != ownerID {
		return nil, errors.New("not found")
	}
	return data, nil
}

// Helper: update item bypassing user check (for shared write access)
func (s *SharingService) updateItemAsOwner(ctx context.Context, bucket, itemID, ownerID string, data map[string]interface{}) error {
	collection := "personal-" + bucket
	doc, err := s.client.Collection(collection).Doc(itemID).Get(ctx)
	if err != nil {
		return err
	}
	existing := doc.Data()
	if existing["user_id"] != ownerID {
		return errors.New("not found")
	}

	// Preserve ownership
	data["user_id"] = ownerID
	data["created_at"] = existing["created_at"]
	data["updated_at"] = time.Now()

	_, err = s.client.Collection(collection).Doc(itemID).Set(ctx, data)
	return err
}
