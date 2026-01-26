package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"google.golang.org/api/iterator"

	"github.com/yourusername/my-app-backend/services/game-service/auth"
	"github.com/yourusername/my-app-backend/services/game-service/buckets"
	"github.com/yourusername/my-app-backend/services/game-service/users"
)

// Test infrastructure

func setupTestFirestore(t *testing.T) *firestore.Client {
	// Port set by test.sh via FIRESTORE_EMULATOR_HOST env var
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "michal-playground-2026")
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	return client
}

func cleanupTestData(ctx context.Context, client *firestore.Client) {
	iter := client.Collection("personal-test-data").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)
}

func setupTestRouter(client *firestore.Client) http.Handler {
	r := chi.NewRouter()

	mockAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			var userID, email, name string
			if token == "user-a-token" {
				userID = "user-a"
				email = "user-a@test.com"
				name = "User A"
			} else if token == "user-b-token" {
				userID = "user-b"
				email = "user-b@test.com"
				name = "User B"
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", userID)
			ctx = context.WithValue(ctx, "user_email", email)
			ctx = context.WithValue(ctx, "user_name", name)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	configs := []buckets.BucketConfig{
		{Name: "test-data", Type: buckets.PersonalBucket},
	}
	buckets.RegisterBucketRoutes(r, client, configs, mockAuth)

	// Also register open bucket routes for /buckets/mine/*
	buckets.RegisterOpenBucketRoutes(r, client, mockAuth)

	// Register user routes
	users.RegisterRoutes(r, client, mockAuth)

	return r
}

type testResponse struct {
	StatusCode int
	Body       map[string]interface{}
}

func doRequest(server *httptest.Server, method, path string, data map[string]interface{}, token string) testResponse {
	var body *bytes.Buffer
	if data != nil {
		jsonData, _ := json.Marshal(data)
		body = bytes.NewBuffer(jsonData)
	} else {
		body = bytes.NewBuffer(nil)
	}

	req, _ := http.NewRequest(method, server.URL+path, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return testResponse{StatusCode: resp.StatusCode, Body: result}
}

// Tests

func TestCreateItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Offline-first: client must provide ID
	resp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-create-item-1", "title": "Test Item", "count": 42},
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	data := resp.Body["data"].(map[string]interface{})
	if data["user_id"] != "user-a" {
		t.Errorf("Expected user_id 'user-a', got %v", data["user_id"])
	}
	if data["title"] != "Test Item" {
		t.Errorf("Expected title 'Test Item', got %v", data["title"])
	}
	if data["id"] != "test-create-item-1" {
		t.Errorf("Expected id 'test-create-item-1', got %v", data["id"])
	}
}

func TestCreateItemNoAuth(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-no-auth-1", "title": "Test"},
		"") // No token

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

// Offline-first: client must provide ID, server must not generate one
func TestCreateItemRequiresID(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Try to create without ID - should fail
	resp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"title": "No ID Provided"},
		"user-a-token")

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 (bad request), got %d - server should require client-provided ID", resp.StatusCode)
	}
}

func TestGetOwnItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create item
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-get-own-1", "title": "My Item"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Get item
	resp := doRequest(server, "GET", "/buckets/personal/test-data/"+itemID, nil, "user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestGetOtherUserItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates item
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-get-other-1", "title": "User A's Item"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to get it
	resp := doRequest(server, "GET", "/buckets/personal/test-data/"+itemID, nil, "user-b-token")

	if resp.StatusCode != 403 {
		t.Errorf("Expected 403 forbidden, got %d", resp.StatusCode)
	}
}

func TestUpdateOwnItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-update-own-1", "title": "Original"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Update
	resp := doRequest(server, "PUT", "/buckets/personal/test-data/"+itemID,
		map[string]interface{}{"title": "Updated"},
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdateOtherUserItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-update-other-1", "title": "User A's"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to update
	resp := doRequest(server, "PUT", "/buckets/personal/test-data/"+itemID,
		map[string]interface{}{"title": "Hacked"},
		"user-b-token")

	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d", resp.StatusCode)
	}
}

func TestDeleteOwnItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-delete-own-1", "title": "To Delete"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Delete
	resp := doRequest(server, "DELETE", "/buckets/personal/test-data/"+itemID, nil, "user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Verify gone
	getResp := doRequest(server, "GET", "/buckets/personal/test-data/"+itemID, nil, "user-a-token")
	if getResp.StatusCode != 404 {
		t.Errorf("Expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestDeleteOtherUserItem(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates
	createResp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"id": "test-delete-other-1", "title": "Protected"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to delete
	resp := doRequest(server, "DELETE", "/buckets/personal/test-data/"+itemID, nil, "user-b-token")

	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d", resp.StatusCode)
	}
}

// Open bucket tests - /buckets/mine/{anything}

func TestOpenBucketAnyName(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create in "notes" bucket
	resp1 := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "My Note"},
		"user-a-token")

	if resp1.StatusCode != 200 {
		t.Errorf("Expected 200 for notes, got %d", resp1.StatusCode)
	}

	// Create in "todos" bucket - different name, same endpoint pattern
	resp2 := doRequest(server, "POST", "/buckets/mine/todos",
		map[string]interface{}{"task": "Buy milk"},
		"user-a-token")

	if resp2.StatusCode != 200 {
		t.Errorf("Expected 200 for todos, got %d", resp2.StatusCode)
	}

	// Create in "whatever-i-want" bucket
	resp3 := doRequest(server, "POST", "/buckets/mine/whatever-i-want",
		map[string]interface{}{"foo": "bar"},
		"user-a-token")

	if resp3.StatusCode != 200 {
		t.Errorf("Expected 200 for whatever-i-want, got %d", resp3.StatusCode)
	}
}

func TestOpenBucketCRUD(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "Test"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Get
	getResp := doRequest(server, "GET", "/buckets/mine/notes/"+itemID, nil, "user-a-token")
	if getResp.StatusCode != 200 {
		t.Errorf("GET: Expected 200, got %d", getResp.StatusCode)
	}

	// Update
	updateResp := doRequest(server, "PUT", "/buckets/mine/notes/"+itemID,
		map[string]interface{}{"title": "Updated"},
		"user-a-token")
	if updateResp.StatusCode != 200 {
		t.Errorf("PUT: Expected 200, got %d", updateResp.StatusCode)
	}

	// Delete
	deleteResp := doRequest(server, "DELETE", "/buckets/mine/notes/"+itemID, nil, "user-a-token")
	if deleteResp.StatusCode != 200 {
		t.Errorf("DELETE: Expected 200, got %d", deleteResp.StatusCode)
	}
}

func TestOpenBucketUserIsolation(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates
	createResp := doRequest(server, "POST", "/buckets/mine/secrets",
		map[string]interface{}{"secret": "user-a-password"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to read - should fail
	getResp := doRequest(server, "GET", "/buckets/mine/secrets/"+itemID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Expected 403 for cross-user access, got %d", getResp.StatusCode)
	}
}

func TestListPersonalBucket(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	// Clean up list-test collection before testing
	ctx := context.Background()
	iter := client.Collection("personal-list-test").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create 3 items for user A
	for i := 1; i <= 3; i++ {
		doRequest(server, "POST", "/buckets/mine/list-test",
			map[string]interface{}{"title": "Note " + string(rune('0'+i))},
			"user-a-token")
	}

	// Create 2 items for user B
	for i := 1; i <= 2; i++ {
		doRequest(server, "POST", "/buckets/mine/list-test",
			map[string]interface{}{"title": "User B Note"},
			"user-b-token")
	}

	// List user A's items - should get exactly 3
	listResp := doRequest(server, "GET", "/buckets/mine/list-test", nil, "user-a-token")
	if listResp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", listResp.StatusCode)
	}

	items := listResp.Body["data"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items for user A, got %d", len(items))
	}

	// List user B's items - should get exactly 2
	listRespB := doRequest(server, "GET", "/buckets/mine/list-test", nil, "user-b-token")
	itemsB := listRespB.Body["data"].([]interface{})
	if len(itemsB) != 2 {
		t.Errorf("Expected 2 items for user B, got %d", len(itemsB))
	}
}

func TestListWithQueryFilters(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	// Clean up query-test collection before testing
	ctx := context.Background()
	iter := client.Collection("personal-query-test").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create items with different statuses and priorities
	doRequest(server, "POST", "/buckets/mine/query-test",
		map[string]interface{}{"title": "Task 1", "status": "active", "priority": "high"},
		"user-a-token")
	doRequest(server, "POST", "/buckets/mine/query-test",
		map[string]interface{}{"title": "Task 2", "status": "active", "priority": "low"},
		"user-a-token")
	doRequest(server, "POST", "/buckets/mine/query-test",
		map[string]interface{}{"title": "Task 3", "status": "completed", "priority": "high"},
		"user-a-token")
	doRequest(server, "POST", "/buckets/mine/query-test",
		map[string]interface{}{"title": "Task 4", "status": "completed", "priority": "low"},
		"user-a-token")

	// Query with no filters - should get all 4
	allResp := doRequest(server, "GET", "/buckets/mine/query-test", nil, "user-a-token")
	allItems := allResp.Body["data"].([]interface{})
	if len(allItems) != 4 {
		t.Errorf("Expected 4 items with no filter, got %d", len(allItems))
	}

	// Query by status=active - should get 2
	activeResp := doRequest(server, "GET", "/buckets/mine/query-test?status=active", nil, "user-a-token")
	activeItems := activeResp.Body["data"].([]interface{})
	if len(activeItems) != 2 {
		t.Errorf("Expected 2 active items, got %d", len(activeItems))
	}
	for _, item := range activeItems {
		itemMap := item.(map[string]interface{})
		if itemMap["status"] != "active" {
			t.Errorf("Expected status=active, got %v", itemMap["status"])
		}
	}

	// Query by priority=high - should get 2
	highResp := doRequest(server, "GET", "/buckets/mine/query-test?priority=high", nil, "user-a-token")
	highItems := highResp.Body["data"].([]interface{})
	if len(highItems) != 2 {
		t.Errorf("Expected 2 high priority items, got %d", len(highItems))
	}

	// Query by status=active AND priority=high - should get 1
	activeHighResp := doRequest(server, "GET", "/buckets/mine/query-test?status=active&priority=high", nil, "user-a-token")
	activeHighItems := activeHighResp.Body["data"].([]interface{})
	if len(activeHighItems) != 1 {
		t.Errorf("Expected 1 active+high item, got %d", len(activeHighItems))
	}
	if len(activeHighItems) > 0 {
		itemMap := activeHighItems[0].(map[string]interface{})
		if itemMap["title"] != "Task 1" {
			t.Errorf("Expected Task 1, got %v", itemMap["title"])
		}
	}

	// Query by status=pending - should get 0
	pendingResp := doRequest(server, "GET", "/buckets/mine/query-test?status=pending", nil, "user-a-token")
	pendingItems := getDataArray(pendingResp.Body)
	if len(pendingItems) != 0 {
		t.Errorf("Expected 0 pending items, got %d", len(pendingItems))
	}

	// User B cannot see User A's items even with filters
	userBResp := doRequest(server, "GET", "/buckets/mine/query-test?status=active", nil, "user-b-token")
	userBItems := getDataArray(userBResp.Body)
	if len(userBItems) != 0 {
		t.Errorf("User B should not see User A's items, got %d", len(userBItems))
	}
}

// Helper to safely get data array from response (handles nil)
func getDataArray(body map[string]interface{}) []interface{} {
	data := body["data"]
	if data == nil {
		return []interface{}{}
	}
	arr, ok := data.([]interface{})
	if !ok {
		return []interface{}{}
	}
	return arr
}

// doRequestArray sends a request with a JSON array body
func doRequestArray(server *httptest.Server, method, path string, data []map[string]interface{}, token string) testResponse {
	jsonData, _ := json.Marshal(data)
	body := bytes.NewBuffer(jsonData)

	req, _ := http.NewRequest(method, server.URL+path, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return testResponse{StatusCode: resp.StatusCode, Body: result}
}

func TestBatchSync(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	// Clean up batch-test collection before testing
	ctx := context.Background()
	iter := client.Collection("personal-batch-test").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Batch create 3 new items (no IDs - server generates)
	records := []map[string]interface{}{
		{"title": "Note 1", "status": "active"},
		{"title": "Note 2", "status": "pending"},
		{"title": "Note 3", "status": "active"},
	}

	resp := doRequestArray(server, "POST", "/buckets/mine/batch-test/batch", records, "user-a-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	results := resp.Body["data"].([]interface{})
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// All should be "created"
	for i, r := range results {
		result := r.(map[string]interface{})
		if result["status"] != "created" {
			t.Errorf("Result %d: expected status 'created', got %v", i, result["status"])
		}
		if result["id"] == nil || result["id"] == "" {
			t.Errorf("Result %d: expected id to be set", i)
		}
	}

	// Verify items exist in database
	listResp := doRequest(server, "GET", "/buckets/mine/batch-test", nil, "user-a-token")
	items := listResp.Body["data"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items in list, got %d", len(items))
	}
}

func TestBatchSyncWithExistingIDs(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	// Clean up
	ctx := context.Background()
	iter := client.Collection("personal-batch-update-test").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create an item first
	createResp := doRequest(server, "POST", "/buckets/mine/batch-update-test",
		map[string]interface{}{"title": "Original", "count": 1},
		"user-a-token")
	existingID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Batch with mix of new and existing
	records := []map[string]interface{}{
		{"id": existingID, "title": "Updated", "count": 99}, // Update existing
		{"title": "New Item", "count": 1},                   // Create new
	}

	resp := doRequestArray(server, "POST", "/buckets/mine/batch-update-test/batch", records, "user-a-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	results := resp.Body["data"].([]interface{})

	// First should be "updated"
	first := results[0].(map[string]interface{})
	if first["status"] != "updated" {
		t.Errorf("Expected 'updated' for existing item, got %v", first["status"])
	}
	if first["id"] != existingID {
		t.Errorf("Expected id %s, got %v", existingID, first["id"])
	}

	// Second should be "created"
	second := results[1].(map[string]interface{})
	if second["status"] != "created" {
		t.Errorf("Expected 'created' for new item, got %v", second["status"])
	}

	// Verify update was applied
	getResp := doRequest(server, "GET", "/buckets/mine/batch-update-test/"+existingID, nil, "user-a-token")
	data := getResp.Body["data"].(map[string]interface{})
	if data["title"] != "Updated" {
		t.Errorf("Expected title 'Updated', got %v", data["title"])
	}
	if data["count"].(float64) != 99 {
		t.Errorf("Expected count 99, got %v", data["count"])
	}
}

func TestBatchSyncUserIsolation(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	// Clean up
	ctx := context.Background()
	iter := client.Collection("personal-batch-iso-test").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates an item
	createResp := doRequest(server, "POST", "/buckets/mine/batch-iso-test",
		map[string]interface{}{"title": "User A's Item"},
		"user-a-token")
	userAItemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to update User A's item via batch
	records := []map[string]interface{}{
		{"id": userAItemID, "title": "Hacked by B"},
	}

	resp := doRequestArray(server, "POST", "/buckets/mine/batch-iso-test/batch", records, "user-b-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	results := resp.Body["data"].([]interface{})
	result := results[0].(map[string]interface{})

	// Should be "error" with "forbidden"
	if result["status"] != "error" {
		t.Errorf("Expected status 'error', got %v", result["status"])
	}
	if result["error"] != "forbidden" {
		t.Errorf("Expected error 'forbidden', got %v", result["error"])
	}

	// Verify original item unchanged
	getResp := doRequest(server, "GET", "/buckets/mine/batch-iso-test/"+userAItemID, nil, "user-a-token")
	data := getResp.Body["data"].(map[string]interface{})
	if data["title"] != "User A's Item" {
		t.Errorf("Item should be unchanged, got title %v", data["title"])
	}
}

func TestBatchSyncNoAuth(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	records := []map[string]interface{}{
		{"title": "Test"},
	}

	resp := doRequestArray(server, "POST", "/buckets/mine/test/batch", records, "")

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401 without auth, got %d", resp.StatusCode)
	}
}

// Sharing tests

func TestShareItem(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates a note
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "Shared Note", "content": "Hello"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B can't read it yet
	getResp := doRequest(server, "GET", "/sharing/notes/"+itemID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Expected 403 before sharing, got %d", getResp.StatusCode)
	}

	// User A shares with User B
	shareResp := doRequest(server, "POST", "/sharing/notes/"+itemID,
		map[string]interface{}{"user_id": "user-b", "access": "read"},
		"user-a-token")
	if shareResp.StatusCode != 200 {
		t.Errorf("Share failed: %d - %v", shareResp.StatusCode, shareResp.Body)
	}

	// Now User B can read it
	getResp2 := doRequest(server, "GET", "/sharing/notes/"+itemID, nil, "user-b-token")
	if getResp2.StatusCode != 200 {
		t.Errorf("Expected 200 after sharing, got %d", getResp2.StatusCode)
	}
	if getResp2.Body["data"].(map[string]interface{})["title"] != "Shared Note" {
		t.Errorf("Wrong content returned")
	}
}

func TestShareReadOnly(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates and shares with read-only
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "Read Only"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	doRequest(server, "POST", "/sharing/notes/"+itemID,
		map[string]interface{}{"user_id": "user-b", "access": "read"},
		"user-a-token")

	// User B tries to update - should fail
	updateResp := doRequest(server, "PUT", "/sharing/notes/"+itemID,
		map[string]interface{}{"title": "Hacked"},
		"user-b-token")
	if updateResp.StatusCode != 403 {
		t.Errorf("Expected 403 for read-only update, got %d", updateResp.StatusCode)
	}
}

func TestShareWriteAccess(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates and shares with write access
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "Editable"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	doRequest(server, "POST", "/sharing/notes/"+itemID,
		map[string]interface{}{"user_id": "user-b", "access": "write"},
		"user-a-token")

	// User B can update
	updateResp := doRequest(server, "PUT", "/sharing/notes/"+itemID,
		map[string]interface{}{"title": "Updated by B"},
		"user-b-token")
	if updateResp.StatusCode != 200 {
		t.Errorf("Expected 200 for write access update, got %d", updateResp.StatusCode)
	}
}

func TestSharedWithMe(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates and shares
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "For B"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	doRequest(server, "POST", "/sharing/notes/"+itemID,
		map[string]interface{}{"user_id": "user-b", "access": "read"},
		"user-a-token")

	// User B checks what's shared with them
	listResp := doRequest(server, "GET", "/sharing/with-me", nil, "user-b-token")
	if listResp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", listResp.StatusCode)
	}

	sharings := listResp.Body["data"].([]interface{})
	if len(sharings) == 0 {
		t.Errorf("Expected at least one sharing")
	}
}

func TestUnshare(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates and shares
	createResp := doRequest(server, "POST", "/buckets/mine/notes",
		map[string]interface{}{"title": "Temporary"},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	doRequest(server, "POST", "/sharing/notes/"+itemID,
		map[string]interface{}{"user_id": "user-b", "access": "read"},
		"user-a-token")

	// User A unshares
	unshareResp := doRequest(server, "DELETE", "/sharing/notes/"+itemID+"/user-b", nil, "user-a-token")
	if unshareResp.StatusCode != 200 {
		t.Errorf("Unshare failed: %d", unshareResp.StatusCode)
	}

	// User B can no longer read
	getResp := doRequest(server, "GET", "/sharing/notes/"+itemID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Expected 403 after unshare, got %d", getResp.StatusCode)
	}
}

// =========================================================================
// ORG BUCKET TESTS - Collaborative Note Taking
// =========================================================================

// Helper to set up org membership
func setupOrgMember(ctx context.Context, client *firestore.Client, orgID, userID, role string) {
	member := map[string]interface{}{
		"id":        orgID + "-" + userID,
		"org_id":    orgID,
		"user_id":   userID,
		"role":      role,
		"invited_by": "system",
	}
	client.Collection("org-members").Doc(member["id"].(string)).Set(ctx, member)
}

func cleanupOrgData(ctx context.Context, client *firestore.Client) {
	// Clean org-notes
	iter := client.Collection("org-notes").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	// Clean org-projects
	iter = client.Collection("org-projects").Documents(ctx)
	batch = client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	// Clean org-members
	iter = client.Collection("org-members").Documents(ctx)
	batch = client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	// Clean org-sharings
	iter = client.Collection("org-sharings").Documents(ctx)
	batch = client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)
}

// Test: Members can create org notes, guests cannot
func TestOrgNoteCreateByRole(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	// Setup org with different roles
	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "guest")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Member can create
	resp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{"title": "Team Meeting Notes", "content": "Discuss Q4 goals"},
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Member should create: expected 200, got %d - %v", resp.StatusCode, resp.Body)
	}

	// Guest cannot create
	resp2 := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{"title": "Guest Note"},
		"user-b-token")

	if resp2.StatusCode != 403 {
		t.Errorf("Guest should not create: expected 403, got %d", resp2.StatusCode)
	}
}

// Test: Non-member cannot access org data
func TestOrgNoteNonMemberBlocked(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	// User A is member of acme-corp
	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	// User B is NOT a member of acme-corp

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User B tries to create in acme-corp (not a member)
	resp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{"title": "Intruder"},
		"user-b-token")

	if resp.StatusCode != 403 {
		t.Errorf("Non-member should be blocked: expected 403, got %d", resp.StatusCode)
	}
}

// Test: Visibility - Team visibility (default)
func TestOrgNoteVisibilityTeam(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	// Setup: member creates note, guest tries to read
	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "guest")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Member creates note (default visibility: team)
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{"title": "Team Only Note"},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Guest cannot read team-visibility note
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Guest should not read team note: expected 403, got %d", getResp.StatusCode)
	}
}

// Test: Visibility - Org-wide (everyone in org can read)
func TestOrgNoteVisibilityOrgWide(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "guest")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create org-wide visible note
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Company Announcement",
			"visibility": "org-wide",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Guest CAN read org-wide note
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp.StatusCode != 200 {
		t.Errorf("Guest should read org-wide note: expected 200, got %d", getResp.StatusCode)
	}
}

// Test: Visibility - Private (only creator)
func TestOrgNoteVisibilityPrivate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "member")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create private note
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "My Private Draft",
			"visibility": "private",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Creator can read
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-a-token")
	if getResp.StatusCode != 200 {
		t.Errorf("Creator should read private note: expected 200, got %d", getResp.StatusCode)
	}

	// Other member cannot read
	getResp2 := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp2.StatusCode != 403 {
		t.Errorf("Other member should not read private note: expected 403, got %d", getResp2.StatusCode)
	}
}

// Test: Admin can access everything
func TestOrgNoteAdminAccess(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "admin")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Member creates private note
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "My Private Note",
			"visibility": "private",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Admin CAN read private notes
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp.StatusCode != 200 {
		t.Errorf("Admin should read any note: expected 200, got %d", getResp.StatusCode)
	}
}

// Test: Only creator or admin can delete
func TestOrgNoteDeletePermissions(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "member")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates note
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Deletable",
			"visibility": "org-wide",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B (not creator, not admin) cannot delete
	delResp := doRequest(server, "DELETE", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if delResp.StatusCode != 403 {
		t.Errorf("Non-creator should not delete: expected 403, got %d", delResp.StatusCode)
	}

	// User A (creator) can delete
	delResp2 := doRequest(server, "DELETE", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-a-token")
	if delResp2.StatusCode != 200 {
		t.Errorf("Creator should delete: expected 200, got %d", delResp2.StatusCode)
	}
}

// Test: Cross-org isolation (no bleed)
func TestOrgNoteCrossOrgBlocked(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	// User A in acme-corp
	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	// User B in other-corp
	setupOrgMember(ctx, client, "other-corp", "user-b", "admin")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates note in acme-corp
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Acme Secret",
			"visibility": "org-wide",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B (admin of OTHER org) cannot access acme-corp notes
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Cross-org access should be blocked: expected 403, got %d", getResp.StatusCode)
	}
}

// Test: List only shows visible items
func TestOrgNoteListFiltered(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "guest")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create one org-wide, one team-only
	doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Public Note",
			"visibility": "org-wide",
		},
		"user-a-token")

	doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Team Note",
			"visibility": "team",
		},
		"user-a-token")

	// Guest lists - should only see org-wide
	listResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes", nil, "user-b-token")
	if listResp.StatusCode != 200 {
		t.Errorf("List should work: expected 200, got %d", listResp.StatusCode)
		return
	}

	items := listResp.Body["data"].([]interface{})
	if len(items) != 1 {
		t.Errorf("Guest should see 1 note (org-wide only), got %d", len(items))
	}
}

// Test: Config-driven write permissions (projects require manager+)
func TestOrgConfigDrivenPermissions(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	// Member and Manager in same org
	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "manager")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Member tries to create project (org.json says: "write": "manager")
	resp := doRequest(server, "POST", "/org/acme-corp/buckets/projects",
		map[string]interface{}{"name": "Secret Project"},
		"user-a-token")

	if resp.StatusCode != 403 {
		t.Errorf("Member should NOT create projects: expected 403, got %d", resp.StatusCode)
	}

	// Manager CAN create project
	resp2 := doRequest(server, "POST", "/org/acme-corp/buckets/projects",
		map[string]interface{}{"name": "Manager's Project"},
		"user-b-token")

	if resp2.StatusCode != 200 {
		t.Errorf("Manager should create projects: expected 200, got %d - %v", resp2.StatusCode, resp2.Body)
	}
}

// Test: Invite-only visibility with org sharing
func TestOrgNoteInviteOnlySharing(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupOrgData(ctx, client)

	setupOrgMember(ctx, client, "acme-corp", "user-a", "member")
	setupOrgMember(ctx, client, "acme-corp", "user-b", "member")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create invite-only note
	createResp := doRequest(server, "POST", "/org/acme-corp/buckets/notes",
		map[string]interface{}{
			"title":      "Secret Project",
			"visibility": "invite-only",
		},
		"user-a-token")
	noteID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B cannot read yet
	getResp := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp.StatusCode != 403 {
		t.Errorf("Should not access invite-only before share: expected 403, got %d", getResp.StatusCode)
	}

	// User A shares with User B
	shareResp := doRequest(server, "POST", "/org/acme-corp/sharing/notes/"+noteID,
		map[string]interface{}{
			"user_id": "user-b",
			"access":  "read",
		},
		"user-a-token")
	if shareResp.StatusCode != 200 {
		t.Errorf("Share should work: expected 200, got %d - %v", shareResp.StatusCode, shareResp.Body)
	}

	// Now User B can read
	getResp2 := doRequest(server, "GET", "/org/acme-corp/buckets/notes/"+noteID, nil, "user-b-token")
	if getResp2.StatusCode != 200 {
		t.Errorf("Should access after share: expected 200, got %d", getResp2.StatusCode)
	}
}

// =========================================================================
// USER SERVICE TESTS
// =========================================================================

func cleanupUsers(ctx context.Context, client *firestore.Client) {
	iter := client.Collection("users").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)
}

func TestUserGetOrCreate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// First request creates user
	resp := doRequest(server, "GET", "/users/me", nil, "user-a-token")
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	data := resp.Body["data"].(map[string]interface{})
	if data["id"] != "user-a" {
		t.Errorf("Expected id 'user-a', got %v", data["id"])
	}

	// Second request returns same user
	resp2 := doRequest(server, "GET", "/users/me", nil, "user-a-token")
	if resp2.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp2.StatusCode)
	}
}

func TestUserUpdate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create user first
	doRequest(server, "GET", "/users/me", nil, "user-a-token")

	// Update profile
	resp := doRequest(server, "PUT", "/users/me",
		map[string]interface{}{
			"name":     "Alice Updated",
			"bio":      "I love Go!",
			"location": "NYC",
		},
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	data := resp.Body["data"].(map[string]interface{})
	if data["bio"] != "I love Go!" {
		t.Errorf("Expected bio 'I love Go!', got %v", data["bio"])
	}
}

func TestUserNoAuth(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// No token
	resp := doRequest(server, "GET", "/users/me", nil, "")
	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

// =========================================================================
// FRIENDSHIP TESTS
// =========================================================================

func cleanupFriendships(ctx context.Context, client *firestore.Client) {
	// Clean friendships
	iter := client.Collection("friendships").Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)

	// Clean invites
	iter = client.Collection("friend-invites").Documents(ctx)
	batch = client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		batch.Delete(doc.Ref)
	}
	batch.Commit(ctx)
}

func TestFriendInviteFlow(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupFriendships(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create both users first
	doRequest(server, "GET", "/users/me", nil, "user-a-token")
	doRequest(server, "GET", "/users/me", nil, "user-b-token")

	// User A sends invite to User B's email
	resp := doRequest(server, "POST", "/friends/invite",
		map[string]interface{}{"email": "user-b@test.com"},
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Extract token from response
	inviteData := resp.Body["data"].(map[string]interface{})
	token := inviteData["token"].(string)

	// User B can view invite (public endpoint)
	viewResp := doRequest(server, "GET", "/invite/"+token, nil, "")
	if viewResp.StatusCode != 200 {
		t.Errorf("Expected 200 for view invite, got %d", viewResp.StatusCode)
	}

	// User B accepts invite
	acceptResp := doRequest(server, "POST", "/invite/"+token+"/accept", nil, "user-b-token")
	if acceptResp.StatusCode != 200 {
		t.Errorf("Expected 200 for accept, got %d: %v", acceptResp.StatusCode, acceptResp.Body)
	}

	// Now they're friends - User A can see User B's profile
	profileResp := doRequest(server, "GET", "/friends/user-b", nil, "user-a-token")
	if profileResp.StatusCode != 200 {
		t.Errorf("Expected 200 for friend profile, got %d", profileResp.StatusCode)
	}
}

func TestCannotViewNonFriendProfile(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupFriendships(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create both users
	doRequest(server, "GET", "/users/me", nil, "user-a-token")
	doRequest(server, "GET", "/users/me", nil, "user-b-token")

	// User A tries to view User B's profile (not friends)
	resp := doRequest(server, "GET", "/friends/user-b", nil, "user-a-token")
	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d", resp.StatusCode)
	}
}

func TestListFriends(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupFriendships(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create users
	doRequest(server, "GET", "/users/me", nil, "user-a-token")
	doRequest(server, "GET", "/users/me", nil, "user-b-token")

	// No friends yet
	resp := doRequest(server, "GET", "/friends", nil, "user-a-token")
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Become friends
	inviteResp := doRequest(server, "POST", "/friends/invite",
		map[string]interface{}{"email": "user-b@test.com"},
		"user-a-token")
	token := inviteResp.Body["data"].(map[string]interface{})["token"].(string)
	doRequest(server, "POST", "/invite/"+token+"/accept", nil, "user-b-token")

	// Now should have 1 friend
	resp2 := doRequest(server, "GET", "/friends", nil, "user-a-token")
	friends := resp2.Body["data"].([]interface{})
	if len(friends) != 1 {
		t.Errorf("Expected 1 friend, got %d", len(friends))
	}
}

func TestRemoveFriend(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupFriendships(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create users and become friends
	doRequest(server, "GET", "/users/me", nil, "user-a-token")
	doRequest(server, "GET", "/users/me", nil, "user-b-token")
	inviteResp := doRequest(server, "POST", "/friends/invite",
		map[string]interface{}{"email": "user-b@test.com"},
		"user-a-token")
	token := inviteResp.Body["data"].(map[string]interface{})["token"].(string)
	doRequest(server, "POST", "/invite/"+token+"/accept", nil, "user-b-token")

	// Remove friend
	resp := doRequest(server, "DELETE", "/friends/user-b", nil, "user-a-token")
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// No longer friends
	profileResp := doRequest(server, "GET", "/friends/user-b", nil, "user-a-token")
	if profileResp.StatusCode != 403 {
		t.Errorf("Expected 403 after unfriend, got %d", profileResp.StatusCode)
	}
}

// =========================================================================
// MAGIC LINK AUTH TESTS
// =========================================================================

func cleanupMagicLinks(ctx context.Context, client *firestore.Client) {
	for _, coll := range []string{"magic-links", "sessions"} {
		iter := client.Collection(coll).Documents(ctx)
		batch := client.Batch()
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				continue
			}
			batch.Delete(doc.Ref)
		}
		batch.Commit(ctx)
	}
}

func setupTestRouterWithAuth(client *firestore.Client) http.Handler {
	r := chi.NewRouter()

	// Register auth routes
	magicService := auth.RegisterRoutes(r, client, "http://test.local")

	// Combined middleware (local + session)
	authMiddleware := auth.Middleware(auth.AuthConfig{
		Mode:         "local",
		MagicService: magicService,
	})

	// Register other routes
	configs := []buckets.BucketConfig{
		{Name: "test-data", Type: buckets.PersonalBucket},
	}
	buckets.RegisterBucketRoutes(r, client, configs, authMiddleware)
	buckets.RegisterOpenBucketRoutes(r, client, authMiddleware)
	users.RegisterRoutes(r, client, authMiddleware)

	return r
}

func TestMagicLinkFlow(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupMagicLinks(ctx, client)

	server := httptest.NewServer(setupTestRouterWithAuth(client))
	defer server.Close()

	// Request magic link
	resp := doRequestWithHeader(server, "POST", "/auth/magic-link",
		map[string]interface{}{"email": "alice@example.com"},
		"", map[string]string{"X-Dev-Mode": "true"})

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// In dev mode, magic_url is returned
	magicURL, ok := resp.Body["magic_url"].(string)
	if !ok {
		t.Fatal("Expected magic_url in dev mode")
	}

	// Extract token from URL
	parts := strings.Split(magicURL, "token=")
	if len(parts) != 2 {
		t.Fatalf("Invalid magic URL: %s", magicURL)
	}
	magicToken := parts[1]

	// Verify magic link
	verifyResp := doRequest(server, "GET", "/auth/verify?token="+magicToken, nil, "")
	if verifyResp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", verifyResp.StatusCode, verifyResp.Body)
	}

	// Get session token
	sessionToken, ok := verifyResp.Body["token"].(string)
	if !ok {
		t.Fatal("Expected token in response")
	}

	// Use session token to access protected endpoint
	meResp := doRequest(server, "GET", "/users/me", nil, sessionToken)
	if meResp.StatusCode != 200 {
		t.Errorf("Expected 200 with session token, got %d", meResp.StatusCode)
	}

	// Verify user was created with correct email
	userData := meResp.Body["data"].(map[string]interface{})
	if userData["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", userData["email"])
	}
}

func TestMagicLinkExpired(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupMagicLinks(ctx, client)

	server := httptest.NewServer(setupTestRouterWithAuth(client))
	defer server.Close()

	// Try to verify with invalid token
	resp := doRequest(server, "GET", "/auth/verify?token=invalidtoken123", nil, "")
	if resp.StatusCode != 401 {
		t.Errorf("Expected 401 for invalid token, got %d", resp.StatusCode)
	}
}

func TestMagicLinkUsedOnce(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupMagicLinks(ctx, client)

	server := httptest.NewServer(setupTestRouterWithAuth(client))
	defer server.Close()

	// Request magic link
	resp := doRequestWithHeader(server, "POST", "/auth/magic-link",
		map[string]interface{}{"email": "bob@example.com"},
		"", map[string]string{"X-Dev-Mode": "true"})

	magicURL := resp.Body["magic_url"].(string)
	magicToken := strings.Split(magicURL, "token=")[1]

	// Use it once
	verifyResp := doRequest(server, "GET", "/auth/verify?token="+magicToken, nil, "")
	if verifyResp.StatusCode != 200 {
		t.Fatalf("First use should succeed: %d", verifyResp.StatusCode)
	}

	// Try to use again
	verifyResp2 := doRequest(server, "GET", "/auth/verify?token="+magicToken, nil, "")
	if verifyResp2.StatusCode != 401 {
		t.Errorf("Second use should fail: expected 401, got %d", verifyResp2.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupUsers(ctx, client)
	defer cleanupMagicLinks(ctx, client)

	server := httptest.NewServer(setupTestRouterWithAuth(client))
	defer server.Close()

	// Login
	resp := doRequestWithHeader(server, "POST", "/auth/magic-link",
		map[string]interface{}{"email": "carol@example.com"},
		"", map[string]string{"X-Dev-Mode": "true"})
	magicToken := strings.Split(resp.Body["magic_url"].(string), "token=")[1]
	verifyResp := doRequest(server, "GET", "/auth/verify?token="+magicToken, nil, "")
	sessionToken := verifyResp.Body["token"].(string)

	// Session works
	meResp := doRequest(server, "GET", "/users/me", nil, sessionToken)
	if meResp.StatusCode != 200 {
		t.Errorf("Session should work: %d", meResp.StatusCode)
	}

	// Logout (needs Bearer prefix)
	logoutResp := doRequest(server, "POST", "/auth/logout", nil, "Bearer "+sessionToken)
	if logoutResp.StatusCode != 200 {
		t.Errorf("Logout should succeed: %d", logoutResp.StatusCode)
	}

	// Session no longer works
	meResp2 := doRequest(server, "GET", "/users/me", nil, sessionToken)
	if meResp2.StatusCode != 401 {
		t.Errorf("Session should be invalid after logout: expected 401, got %d", meResp2.StatusCode)
	}
}

// =========================================================================
// FILE UPLOAD TESTS
// =========================================================================

func TestUploadFileToItem(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create an item first
	createResp := doRequest(server, "POST", "/buckets/mine/blocks",
		map[string]interface{}{
			"note_id":  "note-123",
			"type":     "speech",
			"position": 1.0,
		},
		"user-a-token")

	if createResp.StatusCode != 200 {
		t.Fatalf("Failed to create block: %d", createResp.StatusCode)
	}
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Upload file to the item
	resp := doFileUpload(server, "/buckets/mine/blocks/"+itemID+"/upload",
		[]byte("fake audio content"),
		"audio/mp3",
		"user-a-token")

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Verify response has file_url
	data := resp.Body["data"].(map[string]interface{})
	if data["file_url"] == nil {
		t.Error("Expected file_url in response")
	}
	if data["mime_type"] != "audio/mp3" {
		t.Errorf("Expected mime_type 'audio/mp3', got %v", data["mime_type"])
	}
}

func TestUploadFileOnlyOwner(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// User A creates an item
	createResp := doRequest(server, "POST", "/buckets/mine/blocks",
		map[string]interface{}{
			"note_id": "note-123",
			"type":    "speech",
		},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// User B tries to upload - should fail
	resp := doFileUpload(server, "/buckets/mine/blocks/"+itemID+"/upload",
		[]byte("hacker audio"),
		"audio/mp3",
		"user-b-token")

	if resp.StatusCode != 403 {
		t.Errorf("Expected 403 for non-owner upload, got %d", resp.StatusCode)
	}
}

func TestUploadFileItemNotFound(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Try to upload to non-existent item
	resp := doFileUpload(server, "/buckets/mine/blocks/nonexistent-id/upload",
		[]byte("audio"),
		"audio/mp3",
		"user-a-token")

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestUploadFileNoAuth(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doFileUpload(server, "/buckets/mine/blocks/some-id/upload",
		[]byte("audio"),
		"audio/mp3",
		"") // No token

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

func TestUploadFileUpdatesItem(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create block
	createResp := doRequest(server, "POST", "/buckets/mine/blocks",
		map[string]interface{}{
			"note_id": "note-123",
			"type":    "speech",
		},
		"user-a-token")
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Upload file
	uploadResp := doFileUpload(server, "/buckets/mine/blocks/"+itemID+"/upload",
		[]byte("audio content"),
		"audio/mp3",
		"user-a-token")

	if uploadResp.StatusCode != 200 {
		t.Fatalf("Upload failed: %d", uploadResp.StatusCode)
	}

	fileURL := uploadResp.Body["data"].(map[string]interface{})["file_url"].(string)

	// Get item and verify file_url is set
	getResp := doRequest(server, "GET", "/buckets/mine/blocks/"+itemID, nil, "user-a-token")
	if getResp.StatusCode != 200 {
		t.Fatalf("Get failed: %d", getResp.StatusCode)
	}

	itemData := getResp.Body["data"].(map[string]interface{})
	if itemData["file_url"] != fileURL {
		t.Errorf("Expected file_url '%s', got %v", fileURL, itemData["file_url"])
	}
}

// TestUploadFilePreservesExistingFields verifies that uploading a file
// does not wipe out existing fields on the item (regression test)
func TestUploadFilePreservesExistingFields(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create block with multiple fields
	createResp := doRequest(server, "POST", "/buckets/mine/blocks",
		map[string]interface{}{
			"note_id":  "note-456",
			"type":     "speech",
			"position": 1.5,
			"content":  "transcription text",
		},
		"user-a-token")

	if createResp.StatusCode != 200 {
		t.Fatalf("Create failed: %d", createResp.StatusCode)
	}
	itemID := createResp.Body["data"].(map[string]interface{})["id"].(string)

	// Upload file (this should add file_url without removing other fields)
	uploadResp := doFileUpload(server, "/buckets/mine/blocks/"+itemID+"/upload",
		[]byte("audio content here"),
		"audio/mp4",
		"user-a-token")

	if uploadResp.StatusCode != 200 {
		t.Fatalf("Upload failed: %d", uploadResp.StatusCode)
	}

	// Get item and verify ALL original fields are preserved
	getResp := doRequest(server, "GET", "/buckets/mine/blocks/"+itemID, nil, "user-a-token")
	if getResp.StatusCode != 200 {
		t.Fatalf("Get failed: %d", getResp.StatusCode)
	}

	itemData := getResp.Body["data"].(map[string]interface{})

	// Verify file_url was added
	if itemData["file_url"] == nil {
		t.Error("Expected file_url to be set after upload")
	}

	// Verify original fields are preserved (not wiped out)
	if itemData["note_id"] != "note-456" {
		t.Errorf("Expected note_id 'note-456', got %v", itemData["note_id"])
	}
	if itemData["type"] != "speech" {
		t.Errorf("Expected type 'speech', got %v", itemData["type"])
	}
	if itemData["position"] == nil {
		t.Error("Expected position to be preserved")
	}
	if itemData["content"] != "transcription text" {
		t.Errorf("Expected content 'transcription text', got %v", itemData["content"])
	}
}

// Helper for file uploads
func doFileUpload(server *httptest.Server, path string, fileContent []byte, mimeType, token string) testResponse {
	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, err := writer.CreateFormFile("file", "test-file")
	if err != nil {
		panic(err)
	}
	part.Write(fileContent)

	// Add mime_type field
	writer.WriteField("mime_type", mimeType)

	writer.Close()

	req, _ := http.NewRequest("POST", server.URL+path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return testResponse{StatusCode: resp.StatusCode, Body: result}
}

// Helper for requests with custom headers
func doRequestWithHeader(server *httptest.Server, method, path string, data map[string]interface{}, token string, headers map[string]string) testResponse {
	var body *bytes.Buffer
	if data != nil {
		jsonData, _ := json.Marshal(data)
		body = bytes.NewBuffer(jsonData)
	} else {
		body = bytes.NewBuffer(nil)
	}

	req, _ := http.NewRequest(method, server.URL+path, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return testResponse{StatusCode: resp.StatusCode, Body: result}
}
