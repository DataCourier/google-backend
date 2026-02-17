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
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return "", errors.New("unauthorized: user_id not found")
	}

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
		if status.Code(err) == codes.NotFound {
			return nil, errors.New("not found")
		}
		return nil, err
	}

	data := doc.Data()
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

	for key, value := range existingData {
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}

	data["id"] = id
	data["user_id"] = userID
	data["created_at"] = existingData["created_at"]
	data["updated_at"] = time.Now()

	// Optimistic locking: if client sends a version, it must match
	if clientVersion, ok := data["version"]; ok {
		serverVersion, _ := toInt64(existingData["version"])
		cv, cvOk := toInt64(clientVersion)
		if cvOk && cv != serverVersion {
			return fmt.Errorf("conflict: record was modified (server version %d, your version %d)", serverVersion, cv)
		}
		data["version"] = serverVersion + 1
	}

	_, err = b.client.Collection(collection).Doc(id).Set(ctx, data)
	return err
}

func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	}
	return 0, false
}

func (b *PersonalBucketImpl) Delete(ctx context.Context, bucketName string, id string) error {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)

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

type ListOptions struct {
	OrderBy  string
	OrderDir firestore.Direction
	Limit    int
	Offset   int
}

type ListResult struct {
	Data  []map[string]interface{} `json:"data"`
	Total int                      `json:"total"`
}

func (b *PersonalBucketImpl) List(ctx context.Context, bucketName string, filters map[string]interface{}, opts *ListOptions) (*ListResult, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, errors.New("unauthorized")
	}

	collection := fmt.Sprintf("personal-%s", bucketName)
	query := b.client.Collection(collection).Where("user_id", "==", userID)

	if filters != nil {
		for key, value := range filters {
			if key == "user_id" {
				continue
			}
			query = query.Where(key, "==", value)
		}
	}

	// Apply ordering and pagination
	paginatedQuery := query
	if opts != nil {
		if opts.OrderBy != "" {
			paginatedQuery = paginatedQuery.OrderBy(opts.OrderBy, opts.OrderDir)
		}
		if opts.Offset > 0 {
			paginatedQuery = paginatedQuery.Offset(opts.Offset)
		}
		if opts.Limit > 0 {
			paginatedQuery = paginatedQuery.Limit(opts.Limit)
		}
	}

	iter := paginatedQuery.Documents(ctx)
	defer iter.Stop()

	var results []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		results = append(results, doc.Data())
	}

	// Count total: if no pagination was applied, total is just len(results).
	// Otherwise, run a separate count query.
	total := len(results)
	if opts != nil && (opts.Limit > 0 || opts.Offset > 0) {
		countIter := query.Select().Documents(ctx)
		defer countIter.Stop()
		total = 0
		for {
			_, err := countIter.Next()
			if err != nil {
				break
			}
			total++
		}
	}

	return &ListResult{Data: results, Total: total}, nil
}

type BatchResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

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

		doc, err := b.client.Collection(collection).Doc(id).Get(ctx)
		exists := err == nil && doc.Exists()

		if exists {
			existingData := doc.Data()
			if existingData["user_id"] != userID {
				results[i] = BatchResult{ID: id, Status: "error", Error: "forbidden"}
				continue
			}
		}

		record["user_id"] = userID
		record["updated_at"] = time.Now()
		if !exists {
			record["created_at"] = time.Now()
		}

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
