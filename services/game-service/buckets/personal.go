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

	// Require client-provided ID (offline-first architecture)
	// Clients generate IDs locally before syncing - server must not generate IDs
	// as it would break the local-server record link
	id, ok := data["id"].(string)
	if !ok || id == "" {
		return "", errors.New("id is required: offline-first clients must provide ID")
	}
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

	// Merge new data with existing data (preserves fields not in the update)
	for key, value := range existingData {
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}

	// Preserve system fields that shouldn't be overwritten
	data["id"] = id
	data["user_id"] = userID
	data["created_at"] = existingData["created_at"]
	data["updated_at"] = time.Now()

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

// BatchResult represents the result of a single record in a batch operation.
type BatchResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "created", "updated", "error"
	Error  string `json:"error,omitempty"`
}

// Batch creates or updates multiple records in a single operation.
// Records with existing IDs are updated, new IDs are created.
func (b *PersonalBucketImpl) Batch(ctx context.Context, bucketName string, records []map[string]interface{}) ([]BatchResult, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)
	results := make([]BatchResult, len(records))

	for i, record := range records {
		id, hasID := record["id"].(string)
		if !hasID || id == "" {
			id = uuid.New().String()
			record["id"] = id
		}

		// Check if record exists
		doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
		exists := err == nil && doc.Exists()

		if exists {
			// Verify ownership for update
			existingData := doc.Data()
			if existingData["user_id"] != userID {
				results[i] = BatchResult{ID: id, Status: "error", Error: "forbidden"}
				continue
			}
		}

		// Set metadata
		record["user_id"] = userID
		record["updated_at"] = time.Now()
		if !exists {
			record["created_at"] = time.Now()
		}

		// Save
		_, err = b.client.Collection(collection).Doc(id).Set(ctx, record)
		if err != nil {
			results[i] = BatchResult{ID: id, Status: "error", Error: err.Error()}
		} else if exists {
			results[i] = BatchResult{ID: id, Status: "updated"}
		} else {
			results[i] = BatchResult{ID: id, Status: "created"}
		}
	}

	return results, nil
}
