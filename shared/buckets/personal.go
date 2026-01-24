package buckets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PersonalBucketImpl struct {
	client *firestore.Client
}

func NewPersonalBucket(client *firestore.Client) *PersonalBucketImpl {
	return &PersonalBucketImpl{client: client}
}

func (b *PersonalBucketImpl) Create(ctx context.Context, bucketName string, data map[string]interface{}) (string, error) {
	// Extract user_id from context (set by auth middleware)
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return "", errors.New("unauthorized: user_id not found")
	}

	id := uuid.New().String()
	data["id"] = id
	data["user_id"] = userID
	data["created_at"] = time.Now()
	data["updated_at"] = time.Now()

	collection := fmt.Sprintf("personal-%s", bucketName)
	_, err := b.client.Collection(collection).Doc(id).Set(ctx, data)

	return id, err
}

func (b *PersonalBucketImpl) Get(ctx context.Context, bucketName string, id string) (map[string]interface{}, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		// Check if document doesn't exist
		if status.Code(err) == codes.NotFound {
			return nil, errors.New("not found")
		}
		return nil, err
	}

	data := doc.Data()

	// Enforce user isolation
	if data["user_id"] != userID {
		return nil, errors.New("forbidden: not your data")
	}

	return data, nil
}

func (b *PersonalBucketImpl) Update(ctx context.Context, bucketName string, id string, data map[string]interface{}) error {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)

	// Verify ownership
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errors.New("not found")
		}
		return err
	}

	existingData := doc.Data()
	if existingData["user_id"] != userID {
		return errors.New("forbidden: not your data")
	}

	// Update fields
	data["updated_at"] = time.Now()
	data["user_id"] = userID // Prevent changing owner

	_, err = b.client.Collection(collection).Doc(id).Set(ctx, data)
	return err
}

func (b *PersonalBucketImpl) Delete(ctx context.Context, bucketName string, id string) error {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)

	// Verify ownership
	doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errors.New("not found")
		}
		return err
	}

	data := doc.Data()
	if data["user_id"] != userID {
		return errors.New("forbidden: not your data")
	}

	_, err = b.client.Collection(collection).Doc(id).Delete(ctx)
	return err
}

// List returns items in a personal bucket for the current user.
// Supports optional filters for equality queries.
// Example: filters = {"status": "active", "priority": "5"}
func (b *PersonalBucketImpl) List(ctx context.Context, bucketName string, filters ...map[string]interface{}) ([]map[string]interface{}, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)

	// Start with user_id filter (always required)
	query := b.client.Collection(collection).Where("user_id", "==", userID)

	// Apply additional filters if provided
	if len(filters) > 0 && filters[0] != nil {
		for key, value := range filters[0] {
			// Skip internal fields
			if key == "user_id" {
				continue
			}
			query = query.Where(key, "==", value)
		}
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var results []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break // End of iteration or error
		}
		results = append(results, doc.Data())
	}

	return results, nil
}
