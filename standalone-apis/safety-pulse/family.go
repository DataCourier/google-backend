package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"

	"github.com/yourusername/safety-pulse/buckets"
)

func registerFamilyRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/family", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/", createFamilyHandler(client))
		r.Post("/join", joinFamilyHandler(client))
		r.Get("/", familyStatusHandler(client))
	})
}

// POST /family — create a new family (org)
func createFamilyHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value("user_id").(string)
		if userID == "" {
			buckets.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		var req struct {
			FamilyName string `json:"family_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FamilyName == "" {
			buckets.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "family_name is required"})
			return
		}

		orgID := uuid.New().String()
		now := time.Now().UTC()

		// Create the org record
		_, err := client.Collection("orgs").Doc(orgID).Set(r.Context(), map[string]interface{}{
			"id":         orgID,
			"name":       req.FamilyName,
			"created_by": userID,
			"created_at": now,
		})
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create family: " + err.Error()})
			return
		}

		// Add creator as owner
		memberID := orgID + "-" + userID
		_, err = client.Collection("org-members").Doc(memberID).Set(r.Context(), map[string]interface{}{
			"id":         memberID,
			"org_id":     orgID,
			"user_id":    userID,
			"role":       "owner",
			"invited_by": "system",
			"joined_at":  now,
		})
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add owner: " + err.Error()})
			return
		}

		// Generate invite token
		token := generateInviteToken()
		_, err = client.Collection("family-invites").Doc(token).Set(r.Context(), map[string]interface{}{
			"token":      token,
			"org_id":     orgID,
			"created_by": userID,
			"created_at": now,
			"role":       "member", // invitees join as members (beacons)
		})
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite: " + err.Error()})
			return
		}

		buckets.RespondJSON(w, http.StatusCreated, map[string]interface{}{
			"org_id":       orgID,
			"family_name":  req.FamilyName,
			"invite_token": token,
		})
	}
}

// POST /family/join — join a family via invite token
func joinFamilyHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value("user_id").(string)
		if userID == "" {
			buckets.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		var req struct {
			InviteToken string `json:"invite_token"`
			DeviceName  string `json:"device_name"`
			DeviceModel string `json:"device_model"`
			OSVersion   string `json:"os_version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InviteToken == "" {
			buckets.RespondJSON(w, http.StatusBadRequest, map[string]string{"error": "invite_token is required"})
			return
		}

		// Look up invite
		inviteDoc, err := client.Collection("family-invites").Doc(req.InviteToken).Get(r.Context())
		if err != nil {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "invalid invite token"})
			return
		}
		invite := inviteDoc.Data()
		orgID := invite["org_id"].(string)
		role := invite["role"].(string)

		// Check if user is already a member
		memberID := orgID + "-" + userID
		existingDoc, err := client.Collection("org-members").Doc(memberID).Get(r.Context())
		if err == nil && existingDoc.Exists() {
			buckets.RespondJSON(w, http.StatusConflict, map[string]string{"error": "already a member of this family"})
			return
		}

		now := time.Now().UTC()

		// Add user as member
		_, err = client.Collection("org-members").Doc(memberID).Set(r.Context(), map[string]interface{}{
			"id":         memberID,
			"org_id":     orgID,
			"user_id":    userID,
			"role":       role,
			"invited_by": invite["created_by"],
			"joined_at":  now,
		})
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to join family: " + err.Error()})
			return
		}

		// Create beacon record in org-beacons
		orgBucket := buckets.NewOrgBucket(client, buckets.VisibilityOrgWide)
		beaconID, err := orgBucket.Create(r.Context(), orgID, "beacons", map[string]interface{}{
			"device_name":  req.DeviceName,
			"device_model": req.DeviceModel,
			"os_version":   req.OSVersion,
			"push_token":   "",
			"last_seen_at": now,
		})
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create beacon: " + err.Error()})
			return
		}

		// Get family name
		orgDoc, _ := client.Collection("orgs").Doc(orgID).Get(r.Context())
		familyName := ""
		if orgDoc != nil && orgDoc.Exists() {
			if name, ok := orgDoc.Data()["name"].(string); ok {
				familyName = name
			}
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"org_id":      orgID,
			"beacon_id":   beaconID,
			"family_name": familyName,
		})
	}
}

// GET /family — guardian dashboard: all beacons with latest ping
func familyStatusHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value("user_id").(string)
		if userID == "" {
			buckets.RespondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		// Find user's org membership
		iter := client.Collection("org-members").Where("user_id", "==", userID).Documents(r.Context())
		membership, err := iter.Next()
		if err == iterator.Done {
			buckets.RespondJSON(w, http.StatusNotFound, map[string]string{"error": "not a member of any family"})
			return
		}
		if err != nil {
			buckets.RespondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		iter.Stop()

		memberData := membership.Data()
		orgID := memberData["org_id"].(string)

		// Get family name
		orgDoc, err := client.Collection("orgs").Doc(orgID).Get(r.Context())
		familyName := ""
		if err == nil && orgDoc.Exists() {
			if name, ok := orgDoc.Data()["name"].(string); ok {
				familyName = name
			}
		}

		// List all beacons in the org
		beaconIter := client.Collection("org-beacons").Where("org_id", "==", orgID).Documents(r.Context())
		var beaconList []map[string]interface{}
		for {
			doc, err := beaconIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				continue
			}
			beacon := doc.Data()

			// Get latest ping for this beacon
			beaconID, _ := beacon["id"].(string)
			if beaconID != "" {
				pingIter := client.Collection("org-pings").
					Where("beacon_id", "==", beaconID).
					Documents(r.Context())

				// Find latest ping manually (avoids composite index requirement)
				var latestPing map[string]interface{}
				var latestTime time.Time
				for {
					pingDoc, err := pingIter.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						continue
					}
					ping := pingDoc.Data()
					if ct, ok := ping["created_at"].(time.Time); ok {
						if latestPing == nil || ct.After(latestTime) {
							latestPing = ping
							latestTime = ct
						}
					} else {
						// First ping wins if no timestamp
						if latestPing == nil {
							latestPing = ping
						}
					}
				}
				pingIter.Stop()

				if latestPing != nil {
					beacon["battery_level"] = latestPing["battery_level"]
					beacon["charging_state"] = latestPing["charging_state"]
					beacon["step_count"] = latestPing["step_count"]
					beacon["latitude"] = latestPing["latitude"]
					beacon["longitude"] = latestPing["longitude"]
					beacon["ping_source"] = latestPing["ping_source"]
				}
			}

			beaconList = append(beaconList, beacon)
		}

		if beaconList == nil {
			beaconList = []map[string]interface{}{}
		}

		buckets.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"family_name": familyName,
			"org_id":      orgID,
			"beacons":     beaconList,
		})
	}
}

func generateInviteToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
