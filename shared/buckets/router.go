package buckets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
)

var (
	discoveredBuckets = make(map[string]bool)
	discoveredMu      sync.Mutex
	bucketsFile       = "buckets.yaml"
)

func RegisterBucketRoutes(r chi.Router, client *firestore.Client, configs []BucketConfig, authMiddleware func(http.Handler) http.Handler) {
	personalBucket := NewPersonalBucket(client)

	for _, config := range configs {
		if config.Type == PersonalBucket {
			registerPersonalBucketRoutes(r, config.Name, personalBucket, authMiddleware)
		}
	}
}

// RegisterOpenBucketRoutes registers /buckets/mine/{bucket} - any bucket name allowed
// Use for development. For production, use RegisterBucketRoutes with explicit configs.
// Discovered bucket names are written to buckets.yaml for easy production lockdown.
func RegisterOpenBucketRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	bucket := NewPersonalBucket(client)

	// Load existing discovered buckets
	loadDiscoveredBuckets()

	r.Route("/buckets/mine", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/{bucket}", openListHandler(bucket))
		r.Post("/{bucket}", openCreateHandler(bucket))
		r.Get("/{bucket}/{id}", openGetHandler(bucket))
		r.Put("/{bucket}/{id}", openUpdateHandler(bucket))
		r.Delete("/{bucket}/{id}", openDeleteHandler(bucket))
	})

	// Sharing routes
	sharing := NewSharingService(client, bucket)
	registerSharingRoutes(r, sharing, authMiddleware)
}

func registerSharingRoutes(r chi.Router, sharing *SharingService, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/sharing", func(r chi.Router) {
		r.Use(authMiddleware)

		// Share an item: POST /sharing/{bucket}/{id}
		r.Post("/{bucket}/{id}", shareHandler(sharing))

		// Unshare: DELETE /sharing/{bucket}/{id}/{user}
		r.Delete("/{bucket}/{id}/{user}", unshareHandler(sharing))

		// What's shared with me: GET /sharing/with-me
		r.Get("/with-me", sharedWithMeHandler(sharing))

		// What I've shared: GET /sharing/by-me
		r.Get("/by-me", sharedByMeHandler(sharing))

		// Get shared item: GET /sharing/{bucket}/{id}
		r.Get("/{bucket}/{id}", getSharedItemHandler(sharing))

		// Update shared item: PUT /sharing/{bucket}/{id}
		r.Put("/{bucket}/{id}", updateSharedItemHandler(sharing))
	})
}

func trackBucket(name string) {
	discoveredMu.Lock()
	defer discoveredMu.Unlock()

	if discoveredBuckets[name] {
		return
	}

	discoveredBuckets[name] = true
	saveDiscoveredBuckets()
}

func loadDiscoveredBuckets() {
	data, err := os.ReadFile(bucketsFile)
	if err != nil {
		return // File doesn't exist yet
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && strings.HasPrefix(line, "- ") {
			name := strings.TrimPrefix(line, "- ")
			discoveredBuckets[name] = true
		}
	}
}

func saveDiscoveredBuckets() {
	var lines []string
	lines = append(lines, "# Auto-discovered bucket names (dev mode)")
	lines = append(lines, "# Remove this file or edit to lock down for production")
	lines = append(lines, "buckets:")

	for name := range discoveredBuckets {
		lines = append(lines, "  - "+name)
	}

	os.WriteFile(bucketsFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func registerPersonalBucketRoutes(r chi.Router, bucketName string, bucket *PersonalBucketImpl, authMiddleware func(http.Handler) http.Handler) {
	basePath := fmt.Sprintf("/buckets/personal/%s", bucketName)

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get(basePath, listHandler(bucket, bucketName))
		r.Post(basePath, createHandler(bucket, bucketName))
		r.Get(basePath+"/{id}", getHandler(bucket, bucketName))
		r.Put(basePath+"/{id}", updateHandler(bucket, bucketName))
		r.Delete(basePath+"/{id}", deleteHandler(bucket, bucketName))
	})
}

func listHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract query filters from URL params
		filters := make(map[string]interface{})
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				filters[key] = values[0]
			}
		}

		items, err := bucket.List(r.Context(), bucketName, filters)
		if err != nil {
			if strings.Contains(err.Error(), "unauthorized") {
				respondJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "unauthorized",
					"message": "Authentication required",
				})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": err.Error(),
				})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Success",
			"data":    items,
		})
	}
}

func createHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Invalid JSON",
			})
			return
		}

		id, err := bucket.Create(r.Context(), bucketName, data)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error":   "internal_error",
				"message": err.Error(),
			})
			return
		}

		data["id"] = id
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Created successfully",
			"data":    data,
		})
	}
}

func getHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		data, err := bucket.Get(r.Context(), bucketName, id)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				respondJSON(w, http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "You do not have access to this resource",
				})
			} else if strings.Contains(err.Error(), "not found") {
				respondJSON(w, http.StatusNotFound, map[string]string{
					"error":   "not_found",
					"message": "Resource not found",
				})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": err.Error(),
				})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Success",
			"data":    data,
		})
	}
}

func updateHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Invalid JSON",
			})
			return
		}

		if err := bucket.Update(r.Context(), bucketName, id, data); err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				respondJSON(w, http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "You do not have access to this resource",
				})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": err.Error(),
				})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Updated successfully",
			"data":    data,
		})
	}
}

func deleteHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := bucket.Delete(r.Context(), bucketName, id); err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				respondJSON(w, http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "You do not have access to this resource",
				})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": err.Error(),
				})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Deleted successfully",
			"data":    nil,
		})
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Open handlers - bucket name from URL

func openListHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		listHandler(bucket, bucketName)(w, r)
	}
}

func openCreateHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		createHandler(bucket, bucketName)(w, r)
	}
}

func openGetHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		getHandler(bucket, bucketName)(w, r)
	}
}

func openUpdateHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		updateHandler(bucket, bucketName)(w, r)
	}
}

func openDeleteHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		deleteHandler(bucket, bucketName)(w, r)
	}
}

// Sharing handlers

func shareHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		itemID := chi.URLParam(r, "id")

		var req struct {
			UserID string `json:"user_id"`
			Access string `json:"access"` // "read" or "write"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if req.Access == "" {
			req.Access = SharingAccessRead
		}

		result, err := sharing.Share(r.Context(), bucket, itemID, req.UserID, req.Access)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "bad request") {
				status = http.StatusBadRequest
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Shared successfully",
			"data":    result,
		})
	}
}

func unshareHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		itemID := chi.URLParam(r, "id")
		userID := chi.URLParam(r, "user")

		err := sharing.Unshare(r.Context(), bucket, itemID, userID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "Unshared successfully"})
	}
}

func sharedWithMeHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharings, err := sharing.SharedWithMe(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"data": sharings,
		})
	}
}

func sharedByMeHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharings, err := sharing.SharedByMe(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"data": sharings,
		})
	}
}

func getSharedItemHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		itemID := chi.URLParam(r, "id")

		data, err := sharing.GetSharedItem(r.Context(), bucket, itemID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": data})
	}
}

func updateSharedItemHandler(sharing *SharingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		itemID := chi.URLParam(r, "id")

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		err := sharing.UpdateSharedItem(r.Context(), bucket, itemID, data)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Updated successfully",
			"data":    data,
		})
	}
}
