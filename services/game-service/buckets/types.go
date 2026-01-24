package buckets

import "context"

type BucketType string

const (
	PersonalBucket BucketType = "personal"
	SharedBucket   BucketType = "shared" // Future
)

type BucketConfig struct {
	Name string      // e.g., "test-data"
	Type BucketType
}

type Bucket interface {
	Create(ctx context.Context, bucketName string, data map[string]interface{}) (string, error)
	Get(ctx context.Context, bucketName string, id string) (map[string]interface{}, error)
	Update(ctx context.Context, bucketName string, id string, data map[string]interface{}) error
	Delete(ctx context.Context, bucketName string, id string) error
}
