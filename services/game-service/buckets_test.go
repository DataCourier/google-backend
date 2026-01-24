package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"google.golang.org/api/iterator"

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

	resp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"title": "Test Item", "count": 42},
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
}

func TestCreateItemNoAuth(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupTestData(ctx, client)

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/buckets/personal/test-data",
		map[string]interface{}{"title": "Test"},
		"") // No token

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
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
		map[string]interface{}{"title": "My Item"},
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
		map[string]interface{}{"title": "User A's Item"},
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
		map[string]interface{}{"title": "Original"},
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
		map[string]interface{}{"title": "User A's"},
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
		map[string]interface{}{"title": "To Delete"},
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
		map[string]interface{}{"title": "Protected"},
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
