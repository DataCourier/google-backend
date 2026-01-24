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
