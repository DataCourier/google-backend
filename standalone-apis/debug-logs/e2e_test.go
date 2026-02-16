package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func baseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("BASE_URL")
	if u == "" {
		t.Fatal("BASE_URL not set. Example: BASE_URL=https://debug-logs-xxx.run.app go test -v -run E2E")
	}
	return u
}

func testAPIKey(t *testing.T) string {
	t.Helper()
	k := os.Getenv("API_KEY")
	if k == "" {
		t.Fatal("API_KEY not set")
	}
	return k
}

func authedPost(url, key string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	return http.DefaultClient.Do(req)
}

func authedGet(url, key string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)
	return http.DefaultClient.Do(req)
}

func TestE2E_Health(t *testing.T) {
	resp, err := http.Get(baseURL(t) + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestE2E_Unauthorized(t *testing.T) {
	url := baseURL(t)

	// No key
	body, _ := json.Marshal(map[string]interface{}{"app": "test", "logs": "x"})
	resp, err := http.Post(url+"/log", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("no key: expected 401, got %d", resp.StatusCode)
	}

	// Wrong key
	resp2, err := authedPost(url+"/log", "wrong-key", body)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Errorf("wrong key: expected 401, got %d", resp2.StatusCode)
	}

	// GET without key
	resp3, err := http.Get(url + "/logs")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 401 {
		t.Errorf("GET /logs no key: expected 401, got %d", resp3.StatusCode)
	}
}

func TestE2E_PostAndGetLog(t *testing.T) {
	url := baseURL(t)
	key := testAPIKey(t)

	body, _ := json.Marshal(map[string]interface{}{
		"app": "e2e-test-app",
		"logs": map[string]interface{}{
			"screen":  "main",
			"clicks":  42,
			"version": "1.0.0",
		},
	})

	resp, err := authedPost(url+"/log", key, body)
	if err != nil {
		t.Fatalf("POST /log failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("POST /log: expected 200, got %d", resp.StatusCode)
	}

	resp, err = authedGet(url+"/log/e2e-test-app", key)
	if err != nil {
		t.Fatalf("GET /log/e2e-test-app failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /log: expected 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	if data["app"] != "e2e-test-app" {
		t.Errorf("app: expected e2e-test-app, got %v", data["app"])
	}

	logs, ok := data["logs"].(map[string]interface{})
	if !ok {
		t.Fatalf("logs should be an object, got %T", data["logs"])
	}
	if logs["screen"] != "main" {
		t.Errorf("logs.screen: expected main, got %v", logs["screen"])
	}
	if logs["clicks"] != float64(42) {
		t.Errorf("logs.clicks: expected 42, got %v", logs["clicks"])
	}

	t.Logf("Log OK: %v", data)
}

func TestE2E_PostOverwrites(t *testing.T) {
	url := baseURL(t)
	key := testAPIKey(t)

	body1, _ := json.Marshal(map[string]interface{}{"app": "e2e-overwrite", "logs": "version1"})
	authedPost(url+"/log", key, body1)

	body2, _ := json.Marshal(map[string]interface{}{"app": "e2e-overwrite", "logs": "version2"})
	authedPost(url+"/log", key, body2)

	resp, _ := authedGet(url+"/log/e2e-overwrite", key)
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	if data["logs"] != "version2" {
		t.Errorf("expected version2, got %v", data["logs"])
	}
}

func TestE2E_GetNotFound(t *testing.T) {
	key := testAPIKey(t)
	resp, err := authedGet(baseURL(t)+"/log/nonexistent-app-xyz", key)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestE2E_PostValidation(t *testing.T) {
	url := baseURL(t)
	key := testAPIKey(t)

	// Missing app
	body, _ := json.Marshal(map[string]interface{}{"logs": "something"})
	resp, err := authedPost(url+"/log", key, body)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("missing app: expected 400, got %d", resp.StatusCode)
	}

	// Invalid JSON
	resp2, err := authedPost(url+"/log", key, []byte("not json"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Errorf("bad json: expected 400, got %d", resp2.StatusCode)
	}
}

func TestE2E_ListLogs(t *testing.T) {
	url := baseURL(t)
	key := testAPIKey(t)

	body, _ := json.Marshal(map[string]interface{}{"app": "e2e-list-test", "logs": "hello"})
	authedPost(url+"/log", key, body)

	resp, err := authedGet(url+"/logs", key)
	if err != nil {
		t.Fatalf("GET /logs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var apps []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&apps)

	if len(apps) == 0 {
		t.Fatal("expected at least 1 app in list")
	}

	found := false
	for _, a := range apps {
		if a["app"] == "e2e-list-test" {
			found = true
		}
	}
	if !found {
		t.Error("e2e-list-test not found in list")
	}

	t.Logf("Listed %d apps", len(apps))
}
