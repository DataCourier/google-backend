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

	"github.com/yourusername/safety-pulse/buckets"
)

// Test infrastructure

func setupTestFirestore(t *testing.T) *firestore.Client {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "michal-playground-2026")
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	return client
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
			switch token {
			case "guardian-token":
				userID = "user-guardian"
				email = "guardian@test.com"
				name = "Guardian"
			case "beacon-token":
				userID = "user-beacon"
				email = "beacon@test.com"
				name = "Beacon"
			case "outsider-token":
				userID = "user-outsider"
				email = "outsider@test.com"
				name = "Outsider"
			default:
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", userID)
			ctx = context.WithValue(ctx, "user_email", email)
			ctx = context.WithValue(ctx, "user_name", name)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	buckets.RegisterOpenBucketRoutes(r, client, mockAuth)
	registerFamilyRoutes(r, client, mockAuth)
	registerBeaconRoutes(r, client, mockAuth)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		buckets.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}

func setupOrgMember(ctx context.Context, client *firestore.Client, orgID, userID, role string) {
	member := map[string]interface{}{
		"id":         orgID + "-" + userID,
		"org_id":     orgID,
		"user_id":    userID,
		"role":       role,
		"invited_by": "system",
	}
	client.Collection("org-members").Doc(member["id"].(string)).Set(ctx, member)
}

func cleanupCollections(ctx context.Context, client *firestore.Client, collections ...string) {
	for _, col := range collections {
		iter := client.Collection(col).Documents(ctx)
		batch := client.Batch()
		count := 0
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				continue
			}
			batch.Delete(doc.Ref)
			count++
		}
		if count > 0 {
			batch.Commit(ctx)
		}
	}
}

type testResponse struct {
	StatusCode int
	Body       map[string]interface{}
}

func doRequest(server *httptest.Server, method, path string, data interface{}, token string) testResponse {
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

func TestHealthEndpoint(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "GET", "/health", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
	if resp.Body["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", resp.Body["status"])
	}
}

func TestOrgBeaconCreate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "org-beacons", "org-members")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Setup: guardian is owner, beacon is member
	setupOrgMember(ctx, client, "family-test", "user-guardian", "owner")
	setupOrgMember(ctx, client, "family-test", "user-beacon", "member")

	// Beacon registers itself
	resp := doRequest(server, "POST", "/org/family-test/buckets/beacons/",
		map[string]interface{}{
			"id":           "beacon-1",
			"device_name":  "Grandma's iPhone",
			"device_model": "iPhone 15",
			"os_version":   "18.2",
			"push_token":   "",
		}, "beacon-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	data := resp.Body["data"].(map[string]interface{})
	if data["id"] == nil || data["id"] == "" {
		t.Error("Expected a generated id, got empty")
	}
	if data["device_name"] != "Grandma's iPhone" {
		t.Errorf("Expected device_name 'Grandma's iPhone', got %v", data["device_name"])
	}
}

func TestOrgBeaconVisibleToGuardian(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "org-beacons", "org-members")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	setupOrgMember(ctx, client, "family-test", "user-guardian", "owner")
	setupOrgMember(ctx, client, "family-test", "user-beacon", "member")

	// Beacon creates itself
	doRequest(server, "POST", "/org/family-test/buckets/beacons/",
		map[string]interface{}{
			"id":           "beacon-1",
			"device_name":  "Grandma's iPhone",
			"device_model": "iPhone 15",
		}, "beacon-token")

	// Guardian can see beacons (org-wide visibility)
	resp := doRequest(server, "GET", "/org/family-test/buckets/beacons/", nil, "guardian-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	data := resp.Body["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("Expected 1 beacon, got %d", len(data))
	}

	beacon := data[0].(map[string]interface{})
	if beacon["device_name"] != "Grandma's iPhone" {
		t.Errorf("Expected 'Grandma's iPhone', got %v", beacon["device_name"])
	}
}

func TestOrgPingPrivateVisibility(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "org-pings", "org-members")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	setupOrgMember(ctx, client, "family-test", "user-guardian", "owner")
	setupOrgMember(ctx, client, "family-test", "user-beacon", "member")

	// Beacon creates a ping
	resp := doRequest(server, "POST", "/org/family-test/buckets/pings/",
		map[string]interface{}{
			"id":             "ping-1",
			"beacon_id":      "beacon-1",
			"battery_level":  72,
			"charging_state": "unplugged",
			"step_count":     3456,
			"latitude":       51.5074,
			"longitude":      -0.1278,
			"ping_source":    "widget_refresh",
		}, "beacon-token")

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Guardian (owner) can see private pings — admin+ bypasses visibility
	resp = doRequest(server, "GET", "/org/family-test/buckets/pings/", nil, "guardian-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	data := resp.Body["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("Expected 1 ping, got %d", len(data))
	}

	ping := data[0].(map[string]interface{})
	if ping["beacon_id"] != "beacon-1" {
		t.Errorf("Expected beacon_id 'beacon-1', got %v", ping["beacon_id"])
	}
}

func TestOrgCrossFamilyBlocked(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "org-beacons", "org-members")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Two separate families
	setupOrgMember(ctx, client, "family-a", "user-beacon", "member")
	setupOrgMember(ctx, client, "family-b", "user-outsider", "owner")

	// Beacon creates in family-a
	doRequest(server, "POST", "/org/family-a/buckets/beacons/",
		map[string]interface{}{
			"id":          "beacon-1",
			"device_name": "Test Device",
		}, "beacon-token")

	// Outsider from family-b cannot see family-a's beacons
	resp := doRequest(server, "GET", "/org/family-a/buckets/beacons/", nil, "outsider-token")

	// Should be forbidden (not a member of family-a)
	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d: %v", resp.StatusCode, resp.Body)
	}
}

func TestOrgNoAuthBlocked(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "GET", "/org/family-test/buckets/beacons/", nil, "")
	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

// Family endpoint tests

func TestCreateFamily(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/family/",
		map[string]interface{}{"family_name": "The Smiths"},
		"guardian-token")

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201, got %d: %v", resp.StatusCode, resp.Body)
	}
	if resp.Body["family_name"] != "The Smiths" {
		t.Errorf("Expected family_name 'The Smiths', got %v", resp.Body["family_name"])
	}
	if resp.Body["org_id"] == nil || resp.Body["org_id"] == "" {
		t.Error("Expected org_id to be set")
	}
	if resp.Body["invite_token"] == nil || resp.Body["invite_token"] == "" {
		t.Error("Expected invite_token to be set")
	}
}

func TestCreateFamilyNoName(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/family/",
		map[string]interface{}{},
		"guardian-token")

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestJoinFamily(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Guardian creates family
	createResp := doRequest(server, "POST", "/family/",
		map[string]interface{}{"family_name": "The Smiths"},
		"guardian-token")

	if createResp.StatusCode != 201 {
		t.Fatalf("Failed to create family: %d %v", createResp.StatusCode, createResp.Body)
	}

	inviteToken := createResp.Body["invite_token"].(string)
	orgID := createResp.Body["org_id"].(string)

	// Beacon joins with invite token
	joinResp := doRequest(server, "POST", "/family/join",
		map[string]interface{}{
			"invite_token": inviteToken,
			"device_name":  "Grandma's iPhone",
			"device_model": "iPhone 15",
			"os_version":   "18.2",
		}, "beacon-token")

	if joinResp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", joinResp.StatusCode, joinResp.Body)
	}
	if joinResp.Body["org_id"] != orgID {
		t.Errorf("Expected org_id %s, got %v", orgID, joinResp.Body["org_id"])
	}
	if joinResp.Body["beacon_id"] == nil || joinResp.Body["beacon_id"] == "" {
		t.Error("Expected beacon_id to be set")
	}
	if joinResp.Body["family_name"] != "The Smiths" {
		t.Errorf("Expected family_name 'The Smiths', got %v", joinResp.Body["family_name"])
	}
}

func TestJoinFamilyInvalidToken(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/family/join",
		map[string]interface{}{
			"invite_token": "bogus-token",
			"device_name":  "Test",
		}, "beacon-token")

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestJoinFamilyDuplicate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create family and join once
	createResp := doRequest(server, "POST", "/family/",
		map[string]interface{}{"family_name": "Test Family"},
		"guardian-token")
	inviteToken := createResp.Body["invite_token"].(string)

	doRequest(server, "POST", "/family/join",
		map[string]interface{}{"invite_token": inviteToken, "device_name": "Phone"},
		"beacon-token")

	// Try to join again — should get conflict
	resp := doRequest(server, "POST", "/family/join",
		map[string]interface{}{"invite_token": inviteToken, "device_name": "Phone"},
		"beacon-token")

	if resp.StatusCode != 409 {
		t.Errorf("Expected 409 conflict, got %d", resp.StatusCode)
	}
}

func TestFamilyStatusAfterJoin(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	// Create family
	createResp := doRequest(server, "POST", "/family/",
		map[string]interface{}{"family_name": "The Smiths"},
		"guardian-token")
	inviteToken := createResp.Body["invite_token"].(string)

	// Beacon joins
	joinResp := doRequest(server, "POST", "/family/join",
		map[string]interface{}{
			"invite_token": inviteToken,
			"device_name":  "Grandma's iPhone",
			"device_model": "iPhone 15",
		}, "beacon-token")
	beaconID := joinResp.Body["beacon_id"].(string)
	orgID := joinResp.Body["org_id"].(string)

	// Beacon sends a ping via org bucket
	doRequest(server, "POST", "/org/"+orgID+"/buckets/pings/",
		map[string]interface{}{
			"id":             "ping-1",
			"beacon_id":      beaconID,
			"battery_level":  85,
			"charging_state": "charging",
			"step_count":     1234,
			"latitude":       51.5,
			"longitude":      -0.1,
			"ping_source":    "widget_refresh",
		}, "beacon-token")

	// Guardian checks family status
	statusResp := doRequest(server, "GET", "/family/", nil, "guardian-token")

	if statusResp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", statusResp.StatusCode, statusResp.Body)
	}
	if statusResp.Body["family_name"] != "The Smiths" {
		t.Errorf("Expected family_name 'The Smiths', got %v", statusResp.Body["family_name"])
	}

	beacons := statusResp.Body["beacons"].([]interface{})
	if len(beacons) != 1 {
		t.Fatalf("Expected 1 beacon, got %d", len(beacons))
	}

	beacon := beacons[0].(map[string]interface{})
	if beacon["device_name"] != "Grandma's iPhone" {
		t.Errorf("Expected device_name 'Grandma's iPhone', got %v", beacon["device_name"])
	}

	// Should have latest ping data merged in
	if beacon["battery_level"] != float64(85) {
		t.Errorf("Expected battery_level 85, got %v", beacon["battery_level"])
	}
}

func TestFamilyStatusNoMembership(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "GET", "/family/", nil, "outsider-token")
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d: %v", resp.StatusCode, resp.Body)
	}
}

// Beacon endpoint tests

// Helper: create a family and join a beacon, returns orgID and beaconID
func setupFamilyWithBeacon(t *testing.T, server *httptest.Server) (string, string) {
	t.Helper()
	createResp := doRequest(server, "POST", "/family/",
		map[string]interface{}{"family_name": "Test Family"},
		"guardian-token")
	if createResp.StatusCode != 201 {
		t.Fatalf("Failed to create family: %d %v", createResp.StatusCode, createResp.Body)
	}
	joinResp := doRequest(server, "POST", "/family/join",
		map[string]interface{}{
			"invite_token": createResp.Body["invite_token"].(string),
			"device_name":  "Test Phone",
			"device_model": "iPhone 15",
		}, "beacon-token")
	if joinResp.StatusCode != 200 {
		t.Fatalf("Failed to join family: %d %v", joinResp.StatusCode, joinResp.Body)
	}
	return joinResp.Body["org_id"].(string), joinResp.Body["beacon_id"].(string)
}

func TestBeaconPing(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	resp := doRequest(server, "POST", "/beacon/"+beaconID+"/ping",
		map[string]interface{}{
			"timestamp":      "2026-02-21T14:30:00Z",
			"battery_level":  72,
			"charging_state": "unplugged",
			"step_count":     3456,
			"latitude":       51.5074,
			"longitude":      -0.1278,
			"ping_source":    "widget_refresh",
		}, "beacon-token")

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201, got %d: %v", resp.StatusCode, resp.Body)
	}
	if resp.Body["id"] == nil || resp.Body["id"] == "" {
		t.Error("Expected ping id to be set")
	}
}

func TestBeaconPingPreservesRawPayload(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Send ping with extra unknown fields
	resp := doRequest(server, "POST", "/beacon/"+beaconID+"/ping",
		map[string]interface{}{
			"battery_level": 50,
			"custom_field":  "should be preserved",
			"nested":        map[string]interface{}{"deep": true},
		}, "beacon-token")

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Read the ping back from Firestore
	pingID := resp.Body["id"].(string)
	doc, err := client.Collection("org-pings").Doc(pingID).Get(ctx)
	if err != nil {
		t.Fatalf("Failed to read ping: %v", err)
	}
	data := doc.Data()

	// Check extracted field
	if data["battery_level"] != float64(50) {
		t.Errorf("Expected battery_level 50, got %v", data["battery_level"])
	}

	// Check raw payload preserved
	payload, ok := data["payload"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected payload to be a map")
	}
	if payload["custom_field"] != "should be preserved" {
		t.Errorf("Expected custom_field in payload, got %v", payload["custom_field"])
	}
	nested, ok := payload["nested"].(map[string]interface{})
	if !ok || nested["deep"] != true {
		t.Error("Expected nested.deep=true in payload")
	}
}

func TestBeaconPingPartialData(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Send minimal ping — only battery level, no location etc.
	resp := doRequest(server, "POST", "/beacon/"+beaconID+"/ping",
		map[string]interface{}{
			"battery_level": 15,
		}, "beacon-token")

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Verify it was saved
	pingID := resp.Body["id"].(string)
	doc, _ := client.Collection("org-pings").Doc(pingID).Get(ctx)
	data := doc.Data()
	if data["battery_level"] != float64(15) {
		t.Errorf("Expected battery_level 15, got %v", data["battery_level"])
	}
	// latitude should be absent (not zero)
	if _, exists := data["latitude"]; exists {
		t.Error("Expected latitude to be absent for partial ping")
	}
}

func TestBeaconPingNotFound(t *testing.T) {
	client := setupTestFirestore(t)
	defer client.Close()

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	resp := doRequest(server, "POST", "/beacon/nonexistent-beacon/ping",
		map[string]interface{}{"battery_level": 50},
		"beacon-token")

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestBeaconListPings(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Send 3 pings
	for i := 0; i < 3; i++ {
		doRequest(server, "POST", "/beacon/"+beaconID+"/ping",
			map[string]interface{}{"battery_level": 100 - i*10},
			"beacon-token")
	}

	// Guardian lists pings
	resp := doRequest(server, "GET", "/beacon/"+beaconID+"/pings", nil, "guardian-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	count := resp.Body["count"].(float64)
	if count != 3 {
		t.Errorf("Expected 3 pings, got %v", count)
	}
}

func TestBeaconGetAndUpdate(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Get beacon
	resp := doRequest(server, "GET", "/beacon/"+beaconID+"/", nil, "beacon-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}
	data := resp.Body["data"].(map[string]interface{})
	if data["device_name"] != "Test Phone" {
		t.Errorf("Expected 'Test Phone', got %v", data["device_name"])
	}

	// Update push token
	resp = doRequest(server, "PUT", "/beacon/"+beaconID+"/",
		map[string]interface{}{"push_token": "apns-token-123"},
		"beacon-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Verify update
	resp = doRequest(server, "GET", "/beacon/"+beaconID+"/", nil, "beacon-token")
	data = resp.Body["data"].(map[string]interface{})
	if data["push_token"] != "apns-token-123" {
		t.Errorf("Expected push_token 'apns-token-123', got %v", data["push_token"])
	}
}

func TestBeaconDelete(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Guardian deletes beacon
	resp := doRequest(server, "DELETE", "/beacon/"+beaconID+"/", nil, "guardian-token")
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Beacon should no longer exist
	resp = doRequest(server, "GET", "/beacon/"+beaconID+"/", nil, "guardian-token")
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestBeaconOutsiderBlocked(t *testing.T) {
	ctx := context.Background()
	client := setupTestFirestore(t)
	defer client.Close()
	defer cleanupCollections(ctx, client, "orgs", "org-members", "family-invites", "org-beacons", "org-pings")

	server := httptest.NewServer(setupTestRouter(client))
	defer server.Close()

	_, beaconID := setupFamilyWithBeacon(t, server)

	// Outsider tries to get beacon
	resp := doRequest(server, "GET", "/beacon/"+beaconID+"/", nil, "outsider-token")
	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d: %v", resp.StatusCode, resp.Body)
	}

	// Outsider tries to list pings
	resp = doRequest(server, "GET", "/beacon/"+beaconID+"/pings", nil, "outsider-token")
	if resp.StatusCode != 403 {
		t.Errorf("Expected 403, got %d: %v", resp.StatusCode, resp.Body)
	}
}
