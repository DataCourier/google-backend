package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func setupClient(t *testing.T) *firestore.Client {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set, skipping integration test")
	}
	client, err := firestore.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func seedDocs(t *testing.T, client *firestore.Client, collection string, docs []map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	for _, d := range docs {
		id := d["id"].(string)
		_, err := client.Collection(collection).Doc(id).Set(ctx, d)
		if err != nil {
			t.Fatalf("failed to seed doc %s: %v", id, err)
		}
	}
}

func cleanupCollection(t *testing.T, client *firestore.Client, collection string) {
	t.Helper()
	ctx := context.Background()
	iter := client.Collection(collection).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		doc.Ref.Delete(ctx)
	}
}

func fetchAllDocs(t *testing.T, client *firestore.Client, collection, dedupeOn, protectField string) []Doc {
	t.Helper()
	ctx := context.Background()
	iter := client.Collection(collection).Documents(ctx)
	defer iter.Stop()
	var docs []Doc
	for {
		snap, err := iter.Next()
		if err != nil {
			break
		}
		data := snap.Data()
		d := Doc{ID: snap.Ref.ID}
		if v, ok := data["user_id"].(string); ok {
			d.UserID = v
		}
		if v, ok := data[dedupeOn].(string); ok {
			d.DedupeValue = v
		}
		if v, ok := data[protectField].(string); ok {
			d.ProtectValue = v
		}
		if v, ok := data["created_at"].(time.Time); ok {
			d.CreatedAt = v
		}
		docs = append(docs, d)
	}
	return docs
}

func TestDedupe_RemovesDuplicates(t *testing.T) {
	docs := []Doc{
		{ID: "a1", UserID: "u1", DedupeValue: "vid1", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "a2", UserID: "u1", DedupeValue: "vid1", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "a3", UserID: "u1", DedupeValue: "vid1", CreatedAt: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
	}
	result := findDuplicates(docs)

	if result.DuplicateGroups != 1 {
		t.Errorf("expected 1 duplicate group, got %d", result.DuplicateGroups)
	}
	if len(result.ToDelete) != 2 {
		t.Errorf("expected 2 to delete, got %d", len(result.ToDelete))
	}
	// Should keep the oldest (a1)
	for _, d := range result.ToDelete {
		if d.ID == "a1" {
			t.Error("oldest doc a1 should not be marked for deletion")
		}
	}
}

func TestDedupe_ProtectsNotes(t *testing.T) {
	docs := []Doc{
		{ID: "b1", UserID: "u1", DedupeValue: "vid2", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "b2", UserID: "u1", DedupeValue: "vid2", ProtectValue: "my notes", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	result := findDuplicates(docs)

	if len(result.ToDelete) != 1 {
		t.Fatalf("expected 1 to delete, got %d", len(result.ToDelete))
	}
	if result.ToDelete[0].ID != "b1" {
		t.Errorf("expected b1 (no notes) to be deleted, got %s", result.ToDelete[0].ID)
	}
	if result.Protected != 1 {
		t.Errorf("expected 1 protected, got %d", result.Protected)
	}
}

func TestDedupe_MultipleNotesKeepsAll(t *testing.T) {
	docs := []Doc{
		{ID: "c1", UserID: "u1", DedupeValue: "vid3", ProtectValue: "notes 1", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "c2", UserID: "u1", DedupeValue: "vid3", ProtectValue: "notes 2", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	result := findDuplicates(docs)

	if len(result.ToDelete) != 0 {
		t.Errorf("expected 0 to delete when all have notes, got %d", len(result.ToDelete))
	}
	if result.Protected != 2 {
		t.Errorf("expected 2 protected, got %d", result.Protected)
	}
}

func TestDedupe_DryRunDeletesNothing(t *testing.T) {
	client := setupClient(t)
	collection := "test-dryrun-dedupe"
	cleanupCollection(t, client, collection)
	t.Cleanup(func() { cleanupCollection(t, client, collection) })

	seedDocs(t, client, collection, []map[string]interface{}{
		{"id": "d1", "user_id": "u1", "video_id": "vid4", "created_at": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"id": "d2", "user_id": "u1", "video_id": "vid4", "created_at": time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
	})

	docs := fetchAllDocs(t, client, collection, "video_id", "notes")
	result := findDuplicates(docs)

	// Verify duplicates are found but we don't delete (simulating dry-run — just don't call delete)
	if len(result.ToDelete) != 1 {
		t.Fatalf("expected 1 to delete, got %d", len(result.ToDelete))
	}

	// Verify docs are still there
	remaining := fetchAllDocs(t, client, collection, "video_id", "notes")
	if len(remaining) != 2 {
		t.Errorf("dry run should leave all docs, got %d", len(remaining))
	}
}

func TestDedupe_CrossUserNotGrouped(t *testing.T) {
	docs := []Doc{
		{ID: "e1", UserID: "u1", DedupeValue: "vid5", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "e2", UserID: "u2", DedupeValue: "vid5", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	result := findDuplicates(docs)

	if result.DuplicateGroups != 0 {
		t.Errorf("expected 0 duplicate groups across users, got %d", result.DuplicateGroups)
	}
	if len(result.ToDelete) != 0 {
		t.Errorf("expected 0 to delete across users, got %d", len(result.ToDelete))
	}
}

func TestDedupe_NoDuplicatesNoOp(t *testing.T) {
	docs := []Doc{
		{ID: "f1", UserID: "u1", DedupeValue: "vid6", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "f2", UserID: "u1", DedupeValue: "vid7", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "f3", UserID: "u1", DedupeValue: "vid8", CreatedAt: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
	}
	result := findDuplicates(docs)

	if result.DuplicateGroups != 0 {
		t.Errorf("expected 0 duplicate groups, got %d", result.DuplicateGroups)
	}
	if len(result.ToDelete) != 0 {
		t.Errorf("expected 0 to delete, got %d", len(result.ToDelete))
	}
}

func TestDedupe_IntegrationDeletesCorrectDocs(t *testing.T) {
	client := setupClient(t)
	collection := "test-integration-dedupe"
	cleanupCollection(t, client, collection)
	t.Cleanup(func() { cleanupCollection(t, client, collection) })

	seedDocs(t, client, collection, []map[string]interface{}{
		{"id": "g1", "user_id": "u1", "video_id": "vid9", "created_at": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"id": "g2", "user_id": "u1", "video_id": "vid9", "created_at": time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"id": "g3", "user_id": "u1", "video_id": "vid9", "notes": "important", "created_at": time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
		{"id": "g4", "user_id": "u1", "video_id": "vid10", "created_at": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	})

	docs := fetchAllDocs(t, client, collection, "video_id", "notes")
	result := findDuplicates(docs)

	// vid9 has 3 docs: g3 protected (notes), g1 oldest unprotected kept? No — g3 is protected so all unprotected get deleted
	// g1 and g2 are unprotected, g3 is protected → delete g1 and g2
	if len(result.ToDelete) != 2 {
		t.Fatalf("expected 2 to delete, got %d: %+v", len(result.ToDelete), result.ToDelete)
	}

	// Actually delete
	ctx := context.Background()
	for _, d := range result.ToDelete {
		_, err := client.Collection(collection).Doc(d.ID).Delete(ctx)
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}
	}

	// Verify remaining docs
	remaining := fetchAllDocs(t, client, collection, "video_id", "notes")
	if len(remaining) != 2 {
		t.Errorf("expected 2 remaining docs, got %d", len(remaining))
	}
	ids := map[string]bool{}
	for _, d := range remaining {
		ids[d.ID] = true
	}
	if !ids["g3"] {
		t.Error("g3 (with notes) should have been kept")
	}
	if !ids["g4"] {
		t.Error("g4 (unique vid10) should have been kept")
	}

	// Print for clarity
	fmt.Printf("Remaining: %v\n", ids)
}
