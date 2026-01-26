package buckets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
)

// ForceResyncRequest is the request body for the force-resync endpoint
type ForceResyncRequest struct {
	Bucket string   `json:"bucket"`            // required
	UserID string   `json:"user_id,omitempty"` // optional - specific user
	IDs    []string `json:"ids,omitempty"`     // optional - specific record IDs
}

// RegisterAdminRoutes registers admin endpoints for bucket operations
func RegisterAdminRoutes(r chi.Router, client *firestore.Client, adminMiddleware func(http.Handler) http.Handler) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(adminMiddleware)
		r.Post("/force-resync", forceResyncHandler(client))
	})
}

// forceResyncHandler marks records for re-sync by setting syncedAt = -1
// This signals clients to push their local version on next sync
func forceResyncHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ForceResyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Invalid JSON",
			})
			return
		}

		if req.Bucket == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "bucket is required",
			})
			return
		}

		collection := fmt.Sprintf("personal-%s", req.Bucket)
		ctx := r.Context()
		affected := 0

		if len(req.IDs) > 0 {
			// Update specific records by ID
			for _, id := range req.IDs {
				if err := markForResync(ctx, client, collection, id); err != nil {
					// Log but continue with other records
					fmt.Printf("Warning: failed to mark %s for resync: %v\n", id, err)
					continue
				}
				affected++
			}
		} else if req.UserID != "" {
			// Update all records for a specific user
			query := client.Collection(collection).Where("user_id", "==", req.UserID)
			iter := query.Documents(ctx)
			defer iter.Stop()

			for {
				doc, err := iter.Next()
				if err != nil {
					break // End of iteration
				}
				if err := markForResync(ctx, client, collection, doc.Ref.ID); err != nil {
					fmt.Printf("Warning: failed to mark %s for resync: %v\n", doc.Ref.ID, err)
					continue
				}
				affected++
			}
		} else {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Either user_id or ids must be provided",
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message":  fmt.Sprintf("Marked %d records for re-sync", affected),
			"affected": affected,
		})
	}
}

// markForResync sets syncedAt = -1 on a single document
func markForResync(ctx context.Context, client *firestore.Client, collection, id string) error {
	_, err := client.Collection(collection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "syncedAt", Value: int64(-1)},
	})
	return err
}
