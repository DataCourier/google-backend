package buckets

import (
	"context"
	"fmt"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
)

func testContext(userID string) context.Context {
	return context.WithValue(context.Background(), "user_id", userID)
}

func setupBucket(t *testing.T) *PersonalBucketImpl {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set, skipping integration test")
	}
	client, err := firestore.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return NewPersonalBucket(client)
}

// cleanup deletes all docs in a collection
func cleanup(t *testing.T, bucket *PersonalBucketImpl, bucketName, userID string) {
	t.Helper()
	ctx := testContext(userID)
	collection := fmt.Sprintf("personal-%s", bucketName)
	iter := bucket.client.Collection(collection).Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		doc.Ref.Delete(ctx)
	}
}

func TestBatch_DedupeOn(t *testing.T) {
	bucket := setupBucket(t)
	userID := "test-dedupe-user"
	ctx := testContext(userID)
	bucketName := "test-dedupe"
	cleanup(t, bucket, bucketName, userID)
	t.Cleanup(func() { cleanup(t, bucket, bucketName, userID) })

	// Insert initial records
	records := []map[string]interface{}{
		{"id": "r1", "video_id": "vid_aaa", "title": "First"},
		{"id": "r2", "video_id": "vid_bbb", "title": "Second"},
	}
	results, err := bucket.Batch(ctx, bucketName, records, "video_id")
	if err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}
	for _, r := range results {
		if r.Status != "created" {
			t.Errorf("expected created, got %s for %s", r.Status, r.ID)
		}
	}

	// Insert again with same video_ids but different doc IDs — should be skipped
	dupes := []map[string]interface{}{
		{"id": "r3", "video_id": "vid_aaa", "title": "Dupe of First"},
		{"id": "r4", "video_id": "vid_ccc", "title": "Third (new)"},
	}
	results, err = bucket.Batch(ctx, bucketName, dupes, "video_id")
	if err != nil {
		t.Fatalf("batch dedupe failed: %v", err)
	}
	if results[0].Status != "skipped" {
		t.Errorf("expected skipped for duplicate vid_aaa, got %s", results[0].Status)
	}
	if results[1].Status != "created" {
		t.Errorf("expected created for new vid_ccc, got %s", results[1].Status)
	}

	// Verify total count is 3 (not 4)
	listResult, err := bucket.List(ctx, bucketName, nil, nil)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if listResult.Total != 3 {
		t.Errorf("expected 3 records, got %d", listResult.Total)
	}
}

func TestBatch_DedupeWithinSameBatch(t *testing.T) {
	bucket := setupBucket(t)
	userID := "test-intra-dedupe-user"
	ctx := testContext(userID)
	bucketName := "test-intra-dedupe"
	cleanup(t, bucket, bucketName, userID)
	t.Cleanup(func() { cleanup(t, bucket, bucketName, userID) })

	// Batch with duplicate video_ids within the same request
	records := []map[string]interface{}{
		{"id": "r1", "video_id": "vid_same", "title": "First"},
		{"id": "r2", "video_id": "vid_same", "title": "Duplicate"},
		{"id": "r3", "video_id": "vid_other", "title": "Other"},
	}
	results, err := bucket.Batch(ctx, bucketName, records, "video_id")
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	if results[0].Status != "created" {
		t.Errorf("expected created for first vid_same, got %s", results[0].Status)
	}
	if results[1].Status != "skipped" {
		t.Errorf("expected skipped for intra-batch dupe, got %s", results[1].Status)
	}
	if results[2].Status != "created" {
		t.Errorf("expected created for vid_other, got %s", results[2].Status)
	}
}

func TestBatch_NoDedupeWithoutParam(t *testing.T) {
	bucket := setupBucket(t)
	userID := "test-no-dedupe-user"
	ctx := testContext(userID)
	bucketName := "test-no-dedupe"
	cleanup(t, bucket, bucketName, userID)
	t.Cleanup(func() { cleanup(t, bucket, bucketName, userID) })

	// Without dedupe_on, records with same video_id but different doc IDs both get created
	records := []map[string]interface{}{
		{"id": "r1", "video_id": "vid_x", "title": "First"},
	}
	_, err := bucket.Batch(ctx, bucketName, records, "")
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}

	records2 := []map[string]interface{}{
		{"id": "r2", "video_id": "vid_x", "title": "Second with same video_id"},
	}
	results, err := bucket.Batch(ctx, bucketName, records2, "")
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	if results[0].Status != "created" {
		t.Errorf("without dedupe, expected created, got %s", results[0].Status)
	}

	listResult, err := bucket.List(ctx, bucketName, nil, nil)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if listResult.Total != 2 {
		t.Errorf("expected 2 records without dedupe, got %d", listResult.Total)
	}
}

func TestBatch_DedupeAcrossUsers(t *testing.T) {
	bucket := setupBucket(t)
	bucketName := "test-cross-user"
	userA := "user-a"
	userB := "user-b"
	ctxA := testContext(userA)
	ctxB := testContext(userB)
	cleanup(t, bucket, bucketName, userA)
	cleanup(t, bucket, bucketName, userB)
	t.Cleanup(func() {
		cleanup(t, bucket, bucketName, userA)
		cleanup(t, bucket, bucketName, userB)
	})

	// User A creates a video
	_, err := bucket.Batch(ctxA, bucketName, []map[string]interface{}{
		{"id": "a1", "video_id": "shared_vid", "title": "User A's copy"},
	}, "video_id")
	if err != nil {
		t.Fatalf("batch A failed: %v", err)
	}

	// User B creates the same video_id — should NOT be skipped (different user)
	results, err := bucket.Batch(ctxB, bucketName, []map[string]interface{}{
		{"id": "b1", "video_id": "shared_vid", "title": "User B's copy"},
	}, "video_id")
	if err != nil {
		t.Fatalf("batch B failed: %v", err)
	}
	if results[0].Status != "created" {
		t.Errorf("expected created for user B (different user), got %s", results[0].Status)
	}

	// Each user should see exactly 1 video
	listA, _ := bucket.List(ctxA, bucketName, nil, nil)
	listB, _ := bucket.List(ctxB, bucketName, nil, nil)
	if listA.Total != 1 {
		t.Errorf("user A should have 1 video, got %d", listA.Total)
	}
	if listB.Total != 1 {
		t.Errorf("user B should have 1 video, got %d", listB.Total)
	}
}

func TestList_Pagination(t *testing.T) {
	bucket := setupBucket(t)
	userID := "test-pagination-user"
	ctx := testContext(userID)
	bucketName := "test-pagination"
	cleanup(t, bucket, bucketName, userID)
	t.Cleanup(func() { cleanup(t, bucket, bucketName, userID) })

	// Create 10 records with sequential published dates
	var records []map[string]interface{}
	for i := 0; i < 10; i++ {
		records = append(records, map[string]interface{}{
			"id":        fmt.Sprintf("p%02d", i),
			"video_id":  fmt.Sprintf("vid_%02d", i),
			"published": fmt.Sprintf("2025-01-%02d", i+1),
		})
	}
	_, err := bucket.Batch(ctx, bucketName, records, "")
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}

	// Fetch with limit
	result, err := bucket.List(ctx, bucketName, nil, &ListOptions{
		OrderBy:  "published",
		OrderDir: firestore.Desc,
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(result.Data) != 3 {
		t.Errorf("expected 3 results, got %d", len(result.Data))
	}
	if result.Total != 10 {
		t.Errorf("expected total 10, got %d", result.Total)
	}
	// First result should be the latest (2025-01-10)
	if result.Data[0]["published"] != "2025-01-10" {
		t.Errorf("expected first result published=2025-01-10, got %v", result.Data[0]["published"])
	}

	// Fetch with offset
	result2, err := bucket.List(ctx, bucketName, nil, &ListOptions{
		OrderBy:  "published",
		OrderDir: firestore.Desc,
		Limit:    3,
		Offset:   3,
	})
	if err != nil {
		t.Fatalf("list with offset failed: %v", err)
	}
	if len(result2.Data) != 3 {
		t.Errorf("expected 3 results, got %d", len(result2.Data))
	}
	if result2.Total != 10 {
		t.Errorf("expected total 10, got %d", result2.Total)
	}
	// No overlap with first page
	page1Ids := map[string]bool{}
	for _, d := range result.Data {
		page1Ids[d["video_id"].(string)] = true
	}
	for _, d := range result2.Data {
		if page1Ids[d["video_id"].(string)] {
			t.Errorf("page 2 contains video_id %s from page 1", d["video_id"])
		}
	}
}

func TestList_NoPaginationReturnsAll(t *testing.T) {
	bucket := setupBucket(t)
	userID := "test-no-pagination-user"
	ctx := testContext(userID)
	bucketName := "test-no-pagination"
	cleanup(t, bucket, bucketName, userID)
	t.Cleanup(func() { cleanup(t, bucket, bucketName, userID) })

	var records []map[string]interface{}
	for i := 0; i < 5; i++ {
		records = append(records, map[string]interface{}{
			"id":       fmt.Sprintf("np%d", i),
			"video_id": fmt.Sprintf("vid_%d", i),
		})
	}
	_, err := bucket.Batch(ctx, bucketName, records, "")
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}

	result, err := bucket.List(ctx, bucketName, nil, nil)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(result.Data) != 5 {
		t.Errorf("expected 5 results, got %d", len(result.Data))
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
}
