package buckets

import "context"

type Bucket interface {
	Create(ctx context.Context, bucketName string, data map[string]interface{}) (string, error)
	Get(ctx context.Context, bucketName string, id string) (map[string]interface{}, error)
	Update(ctx context.Context, bucketName string, id string, data map[string]interface{}) error
	Delete(ctx context.Context, bucketName string, id string) error
	List(ctx context.Context, bucketName string, filters ...map[string]interface{}) ([]map[string]interface{}, error)
	Batch(ctx context.Context, bucketName string, records []map[string]interface{}) ([]BatchResult, error)
}
