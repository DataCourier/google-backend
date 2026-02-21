package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourusername/safety-pulse/auth"
	"github.com/yourusername/safety-pulse/buckets"
)

// E2E tests that run against either:
//   - Local: Firestore emulator + httptest server (default)
//   - Production: real Cloud Run instance (set BASE_URL env var)
//
// Local:      ./test.sh -run TestE2E
// Production: BASE_URL=https://safety-pulse-133184350530.us-central1.run.app go test -run TestE2E -v

// testEnv holds the base URL and auth tokens for a test run.
// In production mode, a unique test group suffix isolates data.
type testEnv struct {
	baseURL  string
	suffix   string // unique per run to avoid collisions
	guardian string // auth token for guardian user
	beacon   string // auth token for beacon user
	outsider string // auth token for outsider user
}

func setupE2EEnv(t *testing.T) *testEnv {
	t.Helper()
	suffix := fmt.Sprintf("%d", rand.Intn(1_000_000))

	env := &testEnv{
		suffix:   suffix,
		guardian: "local:e2e-guardian-" + suffix,
		beacon:   "local:e2e-beacon-" + suffix,
		outsider: "local:e2e-outsider-" + suffix,
	}

	if base := os.Getenv("BASE_URL"); base != "" {
		// Production mode — hit real server
		env.baseURL = base
		t.Logf("E2E against production: %s (suffix=%s)", base, suffix)
	} else {
		// Local mode — spin up httptest server with real local: auth
		client := setupTestFirestore(t)
		t.Cleanup(func() { client.Close() })

		r := chi.NewRouter()
		authMW := auth.Middleware(auth.AuthConfig{Mode: "local"})
		buckets.RegisterOpenBucketRoutes(r, client, authMW)
		registerFamilyRoutes(r, client, authMW)
		registerBeaconRoutes(r, client, authMW)
		r.Get("/health", healthHandler(client))

		server := httptest.NewServer(r)
		t.Cleanup(server.Close)
		env.baseURL = server.URL
		t.Logf("E2E against local server: %s (suffix=%s)", env.baseURL, suffix)
	}

	return env
}

func (e *testEnv) do(t *testing.T, method, path string, body interface{}, token string) (int, map[string]interface{}) {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		buf = bytes.NewBuffer(data)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, e.baseURL+path, buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

// --- E2E Tests ---

func TestE2EHealth(t *testing.T) {
	env := setupE2EEnv(t)

	code, body := env.do(t, "GET", "/health", nil, "")
	if code != 200 {
		t.Fatalf("Health check failed: %d %v", code, body)
	}
	if body["status"] != "ok" {
		t.Errorf("Expected status ok, got %v", body["status"])
	}
}

func TestE2ECompleteFlow(t *testing.T) {
	env := setupE2EEnv(t)

	// 1. Guardian creates family
	code, body := env.do(t, "POST", "/family/", map[string]interface{}{
		"family_name": "E2E Family " + env.suffix,
	}, env.guardian)
	if code != 201 {
		t.Fatalf("Create family: expected 201, got %d: %v", code, body)
	}

	inviteToken := body["invite_token"].(string)
	orgID := body["org_id"].(string)
	t.Logf("Created family: org_id=%s", orgID)

	// 2. Beacon joins
	code, body = env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": inviteToken,
		"device_name":  "E2E Beacon " + env.suffix,
		"device_model": "Test Device",
		"os_version":   "1.0",
	}, env.beacon)
	if code != 200 {
		t.Fatalf("Join family: expected 200, got %d: %v", code, body)
	}
	beaconID := body["beacon_id"].(string)
	t.Logf("Beacon joined: beacon_id=%s", beaconID)

	// 3. Beacon sends pings
	for i := 0; i < 3; i++ {
		code, body = env.do(t, "POST", "/beacon/"+beaconID+"/ping", map[string]interface{}{
			"timestamp":      time.Now().UTC().Format(time.RFC3339),
			"battery_level":  90 - i*10,
			"charging_state": "unplugged",
			"step_count":     1000 + i*500,
			"latitude":       51.5 + float64(i)*0.001,
			"longitude":      -0.12,
			"ping_source":    "e2e_test",
		}, env.beacon)
		if code != 201 {
			t.Fatalf("Ping %d: expected 201, got %d: %v", i, code, body)
		}
	}
	t.Log("3 pings sent")

	// 4. Guardian sees dashboard
	code, body = env.do(t, "GET", "/family/", nil, env.guardian)
	if code != 200 {
		t.Fatalf("Family status: expected 200, got %d: %v", code, body)
	}
	if body["family_name"] != "E2E Family "+env.suffix {
		t.Errorf("Expected family name, got %v", body["family_name"])
	}
	beacons := body["beacons"].([]interface{})
	if len(beacons) != 1 {
		t.Fatalf("Expected 1 beacon, got %d", len(beacons))
	}
	beacon := beacons[0].(map[string]interface{})
	if beacon["battery_level"] == nil {
		t.Error("Expected battery_level on dashboard")
	}
	t.Logf("Dashboard: beacon battery=%v", beacon["battery_level"])

	// 5. Guardian lists pings
	code, body = env.do(t, "GET", "/beacon/"+beaconID+"/pings", nil, env.guardian)
	if code != 200 {
		t.Fatalf("List pings: expected 200, got %d: %v", code, body)
	}
	if body["count"].(float64) != 3 {
		t.Errorf("Expected 3 pings, got %v", body["count"])
	}

	// 6. Beacon updates push token
	code, _ = env.do(t, "PUT", "/beacon/"+beaconID+"/", map[string]interface{}{
		"push_token": "e2e-token-" + env.suffix,
	}, env.beacon)
	if code != 200 {
		t.Fatalf("Update beacon: expected 200, got %d", code)
	}

	// 7. Verify update
	code, body = env.do(t, "GET", "/beacon/"+beaconID+"/", nil, env.beacon)
	if code != 200 {
		t.Fatalf("Get beacon: expected 200, got %d", code)
	}
	data := body["data"].(map[string]interface{})
	if data["push_token"] != "e2e-token-"+env.suffix {
		t.Errorf("Expected push_token, got %v", data["push_token"])
	}

	// 8. Outsider blocked
	code, _ = env.do(t, "GET", "/beacon/"+beaconID+"/", nil, env.outsider)
	if code != 403 {
		t.Errorf("Outsider get beacon: expected 403, got %d", code)
	}
	code, _ = env.do(t, "GET", "/beacon/"+beaconID+"/pings", nil, env.outsider)
	if code != 403 {
		t.Errorf("Outsider list pings: expected 403, got %d", code)
	}

	// 9. No auth blocked
	code, _ = env.do(t, "GET", "/family/", nil, "")
	if code != 401 {
		t.Errorf("No auth: expected 401, got %d", code)
	}

	t.Log("E2E full flow passed")
}

func TestE2EJoinDuplicate(t *testing.T) {
	env := setupE2EEnv(t)

	// Create family
	code, body := env.do(t, "POST", "/family/", map[string]interface{}{
		"family_name": "Dup Test " + env.suffix,
	}, env.guardian)
	if code != 201 {
		t.Fatalf("Create: %d %v", code, body)
	}
	token := body["invite_token"].(string)

	// Join once
	code, _ = env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": token,
		"device_name":  "Phone",
	}, env.beacon)
	if code != 200 {
		t.Fatalf("First join: %d", code)
	}

	// Join again — conflict
	code, _ = env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": token,
		"device_name":  "Phone",
	}, env.beacon)
	if code != 409 {
		t.Errorf("Duplicate join: expected 409, got %d", code)
	}
}

func TestE2EInvalidInvite(t *testing.T) {
	env := setupE2EEnv(t)

	code, _ := env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": "bogus-" + env.suffix,
		"device_name":  "Phone",
	}, env.beacon)
	if code != 404 {
		t.Errorf("Invalid invite: expected 404, got %d", code)
	}
}

func TestE2EBeaconNotFound(t *testing.T) {
	env := setupE2EEnv(t)

	code, _ := env.do(t, "POST", "/beacon/nonexistent-"+env.suffix+"/ping",
		map[string]interface{}{"battery_level": 50}, env.beacon)
	if code != 404 {
		t.Errorf("Beacon not found: expected 404, got %d", code)
	}
}

func TestE2EPartialPing(t *testing.T) {
	env := setupE2EEnv(t)

	// Setup family + beacon
	code, body := env.do(t, "POST", "/family/", map[string]interface{}{
		"family_name": "Partial " + env.suffix,
	}, env.guardian)
	if code != 201 {
		t.Fatalf("Create: %d", code)
	}
	code, body = env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": body["invite_token"].(string),
		"device_name":  "Phone",
	}, env.beacon)
	if code != 200 {
		t.Fatalf("Join: %d", code)
	}
	beaconID := body["beacon_id"].(string)

	// Send minimal ping — just battery, no location
	code, body = env.do(t, "POST", "/beacon/"+beaconID+"/ping",
		map[string]interface{}{"battery_level": 15}, env.beacon)
	if code != 201 {
		t.Fatalf("Partial ping: expected 201, got %d: %v", code, body)
	}
	t.Log("Partial ping accepted")
}

func TestE2EDeleteBeacon(t *testing.T) {
	env := setupE2EEnv(t)

	// Setup
	code, body := env.do(t, "POST", "/family/", map[string]interface{}{
		"family_name": "Delete " + env.suffix,
	}, env.guardian)
	if code != 201 {
		t.Fatalf("Create: %d", code)
	}
	code, body = env.do(t, "POST", "/family/join", map[string]interface{}{
		"invite_token": body["invite_token"].(string),
		"device_name":  "Phone",
	}, env.beacon)
	if code != 200 {
		t.Fatalf("Join: %d", code)
	}
	beaconID := body["beacon_id"].(string)

	// Guardian deletes beacon
	code, _ = env.do(t, "DELETE", "/beacon/"+beaconID+"/", nil, env.guardian)
	if code != 200 {
		t.Fatalf("Delete: expected 200, got %d", code)
	}

	// Beacon gone
	code, _ = env.do(t, "GET", "/beacon/"+beaconID+"/", nil, env.guardian)
	if code != 404 {
		t.Errorf("After delete: expected 404, got %d", code)
	}
}
