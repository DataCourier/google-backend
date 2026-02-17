package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"

	"focus-tube/auth"
)

func setupTestRouter(t *testing.T) (chi.Router, *auth.AuthService) {
	t.Helper()

	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set, skipping integration test")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "test-project")
	if err != nil {
		t.Fatalf("firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	svc := auth.NewAuthService(client)
	r := chi.NewRouter()
	auth.RegisterRoutes(r, svc)

	// Add a protected route for middleware testing
	r.Group(func(r chi.Router) {
		r.Use(auth.CombinedMiddleware(svc))
		r.Get("/protected", func(w http.ResponseWriter, r *http.Request) {
			userID := auth.GetUserID(r.Context())
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
		})
	})

	return r, svc
}

func postJSON(router chi.Router, path string, body interface{}, headers ...http.Header) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if len(headers) > 0 {
		for k, v := range headers[0] {
			req.Header[k] = v
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func getWithAuth(router chi.Router, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode json: %v (body: %s)", err, w.Body.String())
	}
	return m
}

func TestFullAuthFlow(t *testing.T) {
	r, _ := setupTestRouter(t)

	email := "test@example.com"

	// Step 1: Request code
	w := postJSON(r, "/auth/request-code", map[string]string{"email": email})
	if w.Code != 200 {
		t.Fatalf("request-code: got %d, body: %s", w.Code, w.Body.String())
	}
	res := decodeJSON(t, w)
	code, ok := res["code"].(string)
	if !ok || len(code) != 6 {
		t.Fatalf("expected 6-digit code, got: %v", res["code"])
	}

	// Step 2: Verify code
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": email, "code": code})
	if w.Code != 200 {
		t.Fatalf("verify-code: got %d, body: %s", w.Code, w.Body.String())
	}
	res = decodeJSON(t, w)
	token, _ := res["token"].(string)
	userID, _ := res["user_id"].(string)
	if token == "" || userID == "" {
		t.Fatalf("expected token and user_id, got: %v", res)
	}

	// Step 3: Access protected route with session token
	w = getWithAuth(r, "/protected", token)
	if w.Code != 200 {
		t.Fatalf("protected: got %d, body: %s", w.Code, w.Body.String())
	}
	res = decodeJSON(t, w)
	if res["user_id"] != userID {
		t.Fatalf("expected user_id %s, got %s", userID, res["user_id"])
	}

	// Step 4: Logout
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	w = postJSON(r, "/auth/logout", nil, h)
	if w.Code != 200 {
		t.Fatalf("logout: got %d, body: %s", w.Code, w.Body.String())
	}

	// Step 5: Token should no longer work
	w = getWithAuth(r, "/protected", token)
	if w.Code != 401 {
		t.Fatalf("expected 401 after logout, got %d", w.Code)
	}
}

func TestLocalTokenMiddleware(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := getWithAuth(r, "/protected", "local:testuser")
	if w.Code != 200 {
		t.Fatalf("local token: got %d, body: %s", w.Code, w.Body.String())
	}
	res := decodeJSON(t, w)
	if res["user_id"] != "testuser" {
		t.Fatalf("expected testuser, got %s", res["user_id"])
	}
}

func TestInvalidToken(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := getWithAuth(r, "/protected", "bogus-token")
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequestCodeMissingEmail(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := postJSON(r, "/auth/request-code", map[string]string{"email": ""})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestVerifyCodeWrongCode(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Request a real code
	w := postJSON(r, "/auth/request-code", map[string]string{"email": "wrong@test.com"})
	if w.Code != 200 {
		t.Fatalf("request-code: got %d", w.Code)
	}

	// Try wrong code
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": "wrong@test.com", "code": "000000"})
	if w.Code != 401 {
		t.Fatalf("expected 401 for wrong code, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestCodeCannotBeReused(t *testing.T) {
	r, _ := setupTestRouter(t)

	email := "reuse@test.com"

	// Request code
	w := postJSON(r, "/auth/request-code", map[string]string{"email": email})
	res := decodeJSON(t, w)
	code := res["code"].(string)

	// First verify — should work
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": email, "code": code})
	if w.Code != 200 {
		t.Fatalf("first verify: got %d", w.Code)
	}

	// Second verify — should fail (code already used)
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": email, "code": code})
	if w.Code != 401 {
		t.Fatalf("expected 401 for reused code, got %d", w.Code)
	}
}

func TestSameEmailGetsSameUser(t *testing.T) {
	r, _ := setupTestRouter(t)

	email := "same@test.com"

	// First login
	w := postJSON(r, "/auth/request-code", map[string]string{"email": email})
	code1 := decodeJSON(t, w)["code"].(string)
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": email, "code": code1})
	userID1 := decodeJSON(t, w)["user_id"].(string)

	// Second login
	w = postJSON(r, "/auth/request-code", map[string]string{"email": email})
	code2 := decodeJSON(t, w)["code"].(string)
	w = postJSON(r, "/auth/verify-code", map[string]string{"email": email, "code": code2})
	userID2 := decodeJSON(t, w)["user_id"].(string)

	if userID1 != userID2 {
		t.Fatalf("same email should get same user_id: %s vs %s", userID1, userID2)
	}
}
