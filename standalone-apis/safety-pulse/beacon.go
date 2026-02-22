package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"

	"github.com/yourusername/safety-pulse/buckets"
)

func registerBeaconRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/beacon/{beaconID}", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/ping", beaconPingHandler(client))
		r.Get("/pings", beaconListPingsHandler(client))
		r.Get("/", beaconGetHandler(client))
		r.Put("/", beaconUpdateHandler(client))
		r.Delete("/", beaconDeleteHandler(client))
	})
}

// POST /beacon/{beaconID}/ping — the core endpoint. Never rejects a ping.
func beaconPingHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		beaconID := chi.URLParam(r, "beaconID")
		userID, _ := r.Context().Value("user_id").(string)

		// Read raw body — save everything
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			// Even if we can't read body, create a minimal ping
			rawBody = []byte("{}")
		}

		// Parse payload — but never fail if it's invalid JSON
		var payload map[string]interface{}
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			payload = map[string]interface{}{
				"_raw":        string(rawBody),
				"_parse_error": err.Error(),
			}
		}

		// Find beacon's org
		orgID, err := findBeaconOrg(r.Context(), client, beaconID)
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "beacon not found"})
			return
		}

		now := time.Now().UTC()
		pingID := uuid.New().String()

		// Build ping document — extract known fields, keep full payload
		pingDoc := map[string]interface{}{
			"id":          pingID,
			"org_id":      orgID,
			"created_by":  userID,
			"beacon_id":   beaconID,
			"received_at": now,
			"payload":     payload,
			"visibility":  "private",
			"created_at":  now,
			"updated_at":  now,
		}

		// Extract known fields to top level for querying
		if v, ok := payload["timestamp"]; ok {
			pingDoc["timestamp"] = v
		}
		if v, ok := payload["battery_level"]; ok {
			pingDoc["battery_level"] = v
		}
		if v, ok := payload["charging_state"]; ok {
			pingDoc["charging_state"] = v
		}
		if v, ok := payload["step_count"]; ok {
			pingDoc["step_count"] = v
		}
		if v, ok := payload["latitude"]; ok {
			pingDoc["latitude"] = v
		}
		if v, ok := payload["longitude"]; ok {
			pingDoc["longitude"] = v
		}
		if v, ok := payload["ping_source"]; ok {
			pingDoc["ping_source"] = v
		}

		// Write ping to Firestore
		_, err = client.Collection("org-pings").Doc(pingID).Set(r.Context(), pingDoc)
		if err != nil {
			log.Printf("ERROR: failed to save ping beacon_id=%s org_id=%s error=%v", beaconID, orgID, err)
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{
				"error":     "failed to save ping",
				"detail":    err.Error(),
				"beacon_id": beaconID,
			})
			return
		}

		log.Printf("PING beacon_id=%s org_id=%s source=%v battery=%v", beaconID, orgID, payload["ping_source"], payload["battery_level"])

		// Update beacon's last_seen_at (best effort — don't fail the ping)
		if _, err := client.Collection("org-beacons").Doc(beaconID).Update(r.Context(), []firestore.Update{
			{Path: "last_seen_at", Value: now},
			{Path: "updated_at", Value: now},
		}); err != nil {
			log.Printf("WARN: failed to update last_seen_at beacon_id=%s error=%v", beaconID, err)
		}

		buckets.RespondJSON(w, http.StatusCreated, map[string]interface{}{
			"id": pingID,
		})
	}
}

// GET /beacon/{beaconID}/pings — list pings with optional time range
func beaconListPingsHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		beaconID := chi.URLParam(r, "beaconID")
		userID, _ := r.Context().Value("user_id").(string)

		// Find beacon's org and verify access
		orgID, err := findBeaconOrg(r.Context(), client, beaconID)
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "beacon not found"})
			return
		}

		// Verify user is in the org
		role, err := getUserOrgRole(r.Context(), client, orgID, userID)
		if err != nil {
			buckets.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this family"})
			return
		}

		// Members can only see their own pings, admin+ can see all
		query := client.Collection("org-pings").Where("beacon_id", "==", beaconID)
		if roleLevel(role) < roleLevel("admin") {
			query = query.Where("created_by", "==", userID)
		}

		// Parse time range filters
		var fromTime, toTime time.Time
		if from := r.URL.Query().Get("from"); from != "" {
			if t, err := time.Parse(time.RFC3339, from); err == nil {
				fromTime = t
			}
		}
		if to := r.URL.Query().Get("to"); to != "" {
			if t, err := time.Parse(time.RFC3339, to); err == nil {
				toTime = t
			}
		}

		// Fetch all pings for beacon, filter in code (avoids composite index requirement)
		iter := query.Documents(r.Context())
		var pings []map[string]interface{}
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				continue
			}
			data := doc.Data()
			if receivedAt, ok := data["received_at"].(time.Time); ok {
				if !fromTime.IsZero() && receivedAt.Before(fromTime) {
					continue
				}
				if !toTime.IsZero() && receivedAt.After(toTime) {
					continue
				}
			}
			pings = append(pings, data)
		}

		if pings == nil {
			pings = []map[string]interface{}{}
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"pings": pings,
			"count": len(pings),
		})
	}
}

// GET /beacon/{beaconID} — get beacon details
func beaconGetHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		beaconID := chi.URLParam(r, "beaconID")
		userID, _ := r.Context().Value("user_id").(string)

		doc, err := client.Collection("org-beacons").Doc(beaconID).Get(r.Context())
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "beacon not found"})
			return
		}

		data := doc.Data()
		orgID, _ := data["org_id"].(string)

		// Verify user is in the org
		if _, err := getUserOrgRole(r.Context(), client, orgID, userID); err != nil {
			buckets.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this family"})
			return
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]interface{}{"data": data})
	}
}

// PUT /beacon/{beaconID} — update beacon (push token, os_version, etc.)
func beaconUpdateHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		beaconID := chi.URLParam(r, "beaconID")
		userID, _ := r.Context().Value("user_id").(string)

		doc, err := client.Collection("org-beacons").Doc(beaconID).Get(r.Context())
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "beacon not found"})
			return
		}

		data := doc.Data()
		orgID, _ := data["org_id"].(string)

		// Verify user is in the org
		if _, err := getUserOrgRole(r.Context(), client, orgID, userID); err != nil {
			buckets.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this family"})
			return
		}

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			buckets.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		// Don't allow overriding protected fields
		delete(updates, "id")
		delete(updates, "org_id")
		delete(updates, "created_by")
		delete(updates, "created_at")
		updates["updated_at"] = time.Now().UTC()

		var firestoreUpdates []firestore.Update
		for k, v := range updates {
			firestoreUpdates = append(firestoreUpdates, firestore.Update{Path: k, Value: v})
		}

		_, err = client.Collection("org-beacons").Doc(beaconID).Update(r.Context(), firestoreUpdates)
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]string{"message": "Updated successfully"})
	}
}

// DELETE /beacon/{beaconID} — remove beacon from family
func beaconDeleteHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		beaconID := chi.URLParam(r, "beaconID")
		userID, _ := r.Context().Value("user_id").(string)

		doc, err := client.Collection("org-beacons").Doc(beaconID).Get(r.Context())
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "beacon not found"})
			return
		}

		data := doc.Data()
		orgID, _ := data["org_id"].(string)

		// Only admin+ or the beacon's creator can delete
		role, err := getUserOrgRole(r.Context(), client, orgID, userID)
		if err != nil {
			buckets.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this family"})
			return
		}
		createdBy, _ := data["created_by"].(string)
		if roleLevel(role) < roleLevel("admin") && createdBy != userID {
			buckets.RespondJSON(w, http.StatusForbidden, map[string]string{"error": "only guardians or the beacon owner can remove a beacon"})
			return
		}

		_, err = client.Collection("org-beacons").Doc(beaconID).Delete(r.Context())
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]string{"message": "Deleted successfully"})
	}
}

// Helper: find which org a beacon belongs to
func findBeaconOrg(ctx context.Context, client *firestore.Client, beaconID string) (string, error) {
	doc, err := client.Collection("org-beacons").Doc(beaconID).Get(ctx)
	if err != nil {
		return "", err
	}
	orgID, _ := doc.Data()["org_id"].(string)
	if orgID == "" {
		return "", fmt.Errorf("beacon has no org_id")
	}
	return orgID, nil
}

// Helper: get user's role in an org
func getUserOrgRole(ctx context.Context, client *firestore.Client, orgID, userID string) (string, error) {
	memberID := orgID + "-" + userID
	doc, err := client.Collection("org-members").Doc(memberID).Get(ctx)
	if err != nil {
		return "", err
	}
	role, _ := doc.Data()["role"].(string)
	return role, nil
}

// roleLevel maps role to numeric level for comparison
func roleLevel(role string) int {
	switch role {
	case "owner":
		return 5
	case "admin":
		return 4
	case "manager":
		return 3
	case "member":
		return 2
	case "guest":
		return 1
	default:
		return 0
	}
}
