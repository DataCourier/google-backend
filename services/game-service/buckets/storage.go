package buckets

import (
	"context"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

// StorageService handles file uploads to Cloud Storage
type StorageService struct {
	client     *storage.Client
	bucketName string
}

// NewStorageService creates a new storage service
func NewStorageService(client *storage.Client, bucketName string) *StorageService {
	return &StorageService{
		client:     client,
		bucketName: bucketName,
	}
}

// UploadResult contains the result of a file upload
type UploadResult struct {
	FileID   string `json:"file_id"`
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// Upload uploads a file to Cloud Storage
// Path format: {userID}/{bucket}/{itemID}/{fileID}.{ext}
func (s *StorageService) Upload(ctx context.Context, userID, bucket, itemID string, data io.Reader, mimeType string, size int64) (*UploadResult, error) {
	// Generate unique file ID
	fileID := uuid.New().String()

	// Determine extension from mime type
	ext := mimeTypeToExt(mimeType)

	// Build object path: users/{userID}/{bucket}/{itemID}/{fileID}.{ext}
	objectPath := fmt.Sprintf("users/%s/%s/%s/%s.%s", userID, bucket, itemID, fileID, ext)

	// Get bucket handle
	bkt := s.client.Bucket(s.bucketName)
	obj := bkt.Object(objectPath)

	// Create writer with metadata
	writer := obj.NewWriter(ctx)
	writer.ContentType = mimeType
	writer.Metadata = map[string]string{
		"user_id": userID,
		"bucket":  bucket,
		"item_id": itemID,
	}

	// Copy data to Cloud Storage
	written, err := io.Copy(writer, data)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to write to storage: %w", err)
	}

	// Close writer to finalize upload
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close storage writer: %w", err)
	}

	// Generate signed URL for download (valid for 7 days)
	// For production, you'd want to use a shorter duration and refresh
	url := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucketName, objectPath)

	return &UploadResult{
		FileID:   fileID,
		URL:      url,
		MimeType: mimeType,
		Size:     written,
	}, nil
}

// GenerateSignedURL generates a signed URL for downloading a file
func (s *StorageService) GenerateSignedURL(ctx context.Context, objectPath string, duration time.Duration) (string, error) {
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(duration),
	}

	url, err := s.client.Bucket(s.bucketName).SignedURL(objectPath, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return url, nil
}

// Delete removes a file from Cloud Storage
func (s *StorageService) Delete(ctx context.Context, objectPath string) error {
	obj := s.client.Bucket(s.bucketName).Object(objectPath)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func mimeTypeToExt(mimeType string) string {
	switch mimeType {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "audio/m4a", "audio/x-m4a":
		return "m4a"
	case "audio/aac":
		return "aac"
	case "audio/ogg":
		return "ogg"
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	default:
		return "bin"
	}
}
