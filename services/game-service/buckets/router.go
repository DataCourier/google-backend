package buckets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
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
		r.Post("/{bucket}/batch", openBatchHandler(bucket))
		r.Get("/{bucket}/{id}", openGetHandler(bucket))
		r.Put("/{bucket}/{id}", openUpdateHandler(bucket))
		r.Delete("/{bucket}/{id}", openDeleteHandler(bucket))

		// File upload endpoint
		r.Post("/{bucket}/{id}/upload", openUploadHandler(bucket))
	})

	// Sharing routes
	sharing := NewSharingService(client, bucket)
	registerSharingRoutes(r, sharing, authMiddleware)

	// Org bucket routes (open - any bucket name)
	registerOpenOrgRoutes(r, client, authMiddleware)
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

func openBatchHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		trackBucket(bucketName)
		batchHandler(bucket, bucketName)(w, r)
	}
}

func batchHandler(bucket *PersonalBucketImpl, bucketName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var records []map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Invalid JSON array",
			})
			return
		}

		// Limit batch size to prevent abuse
		if len(records) > 100 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Batch size exceeds limit of 100 records",
			})
			return
		}

		results, err := bucket.Batch(r.Context(), bucketName, records)
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
			"message": "Batch processed",
			"data":    results,
		})
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

// Org bucket routes

func registerOpenOrgRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/org/{orgID}/buckets/{bucket}", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Post("/", orgCreateHandler(client))
		r.Get("/", orgListHandler(client))
		r.Get("/{id}", orgGetHandler(client))
		r.Put("/{id}", orgUpdateHandler(client))
		r.Delete("/{id}", orgDeleteHandler(client))
	})

	// Org sharing routes
	r.Route("/org/{orgID}/sharing/{bucket}/{id}", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Post("/", orgShareHandler(client))
		r.Delete("/{target}", orgUnshareHandler(client))
	})
}

func orgCreateHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		bucket := NewOrgBucket(client, VisibilityTeam)

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		id, err := bucket.Create(r.Context(), orgID, bucketName, data)
		if err != nil {
			respondJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		data["id"] = id
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Created successfully",
			"data":    data,
		})
	}
}

func orgGetHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")
		bucket := NewOrgBucket(client, VisibilityTeam)

		data, err := bucket.Get(r.Context(), orgID, bucketName, id)
		if err != nil {
			respondJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": data})
	}
}

func orgListHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		bucket := NewOrgBucket(client, VisibilityTeam)

		data, err := bucket.List(r.Context(), orgID, bucketName)
		if err != nil {
			respondJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": data})
	}
}

func orgUpdateHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")
		bucket := NewOrgBucket(client, VisibilityTeam)

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if err := bucket.Update(r.Context(), orgID, bucketName, id, data); err != nil {
			respondJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Updated successfully",
			"data":    data,
		})
	}
}

func orgDeleteHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")
		bucket := NewOrgBucket(client, VisibilityTeam)

		if err := bucket.Delete(r.Context(), orgID, bucketName, id); err != nil {
			respondJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "Deleted successfully"})
	}
}

func orgShareHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		bucketName := chi.URLParam(r, "bucket")
		resourceID := chi.URLParam(r, "id")

		var req struct {
			UserID string `json:"user_id,omitempty"`
			Role   string `json:"role,omitempty"`
			Access string `json:"access"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		userID, _ := r.Context().Value("user_id").(string)

		sharing := OrgSharing{
			ID:             fmt.Sprintf("%s-%s-%s", orgID, resourceID, req.UserID+req.Role),
			OrgID:          orgID,
			ResourceType:   bucketName,
			ResourceID:     resourceID,
			SharedWithUser: req.UserID,
			SharedWithRole: req.Role,
			Access:         req.Access,
			SharedBy:       userID,
		}

		_, err := client.Collection("org-sharings").Doc(sharing.ID).Set(r.Context(), sharing)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Shared successfully",
			"data":    sharing,
		})
	}
}

func orgUnshareHandler(client *firestore.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		resourceID := chi.URLParam(r, "id")
		target := chi.URLParam(r, "target") // user ID or role

		sharingID := fmt.Sprintf("%s-%s-%s", orgID, resourceID, target)
		_, err := client.Collection("org-sharings").Doc(sharingID).Delete(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "Unshared successfully"})
	}
}

func errorStatus(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "forbidden") {
		return http.StatusForbidden
	}
	if strings.Contains(msg, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(msg, "unauthorized") {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

// File upload handler - stores file and updates item with file_url
// POST /buckets/mine/{bucket}/{id}/upload
// Content-Type: multipart/form-data
// Form field: "file" (required)
// Form field: "mime_type" (optional, defaults to file content type)
func openUploadHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		itemID := chi.URLParam(r, "id")
		trackBucket(bucketName)

		// Verify item exists and user owns it
		_, err := bucket.Get(r.Context(), bucketName, itemID)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				respondJSON(w, http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "You do not own this item",
				})
			} else if strings.Contains(err.Error(), "not found") {
				respondJSON(w, http.StatusNotFound, map[string]string{
					"error":   "not_found",
					"message": "Item not found",
				})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": err.Error(),
				})
			}
			return
		}

		// Parse multipart form (32MB max)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Failed to parse multipart form: " + err.Error(),
			})
			return
		}

		// Get uploaded file
		file, header, err := r.FormFile("file")
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Missing 'file' field in form",
			})
			return
		}
		defer file.Close()

		// Get mime type from form or header
		mimeType := r.FormValue("mime_type")
		if mimeType == "" {
			mimeType = header.Header.Get("Content-Type")
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		// Get user ID from context
		userID, ok := r.Context().Value("user_id").(string)
		if !ok || userID == "" {
			respondJSON(w, http.StatusUnauthorized, map[string]string{
				"error":   "unauthorized",
				"message": "User ID not found",
			})
			return
		}

		// Storage bucket configuration
		storageBucket := os.Getenv("GCS_BUCKET")
		if storageBucket == "" {
			storageBucket = "fieldnotes-uploads-dev"
		}

		// Check if we should use mock storage (for testing)
		useMockStorage := os.Getenv("USE_MOCK_STORAGE") == "true" ||
			os.Getenv("FIRESTORE_EMULATOR_HOST") != ""

		var fileURL string
		var fileSize int64 = header.Size

		if useMockStorage {
			// Mock storage mode - don't actually upload to Cloud Storage
			// Just generate a fake URL and store the metadata
			fileURL = fmt.Sprintf("https://storage.googleapis.com/%s/users/%s/%s/%s/mock-file.%s",
				storageBucket, userID, bucketName, itemID, mimeTypeToExt(mimeType))
		} else {
			// Real Cloud Storage mode
			storageClient, err := storage.NewClient(r.Context())
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": "Failed to initialize storage: " + err.Error(),
				})
				return
			}
			defer storageClient.Close()

			// Upload to Cloud Storage
			storageService := NewStorageService(storageClient, storageBucket)
			result, err := storageService.Upload(r.Context(), userID, bucketName, itemID, file, mimeType, header.Size)
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_error",
					"message": "Failed to upload file: " + err.Error(),
				})
				return
			}
			fileURL = result.URL
			fileSize = result.Size
		}

		// Update item with file_url
		updateData := map[string]interface{}{
			"file_url": fileURL,
		}
		if err := bucket.Update(r.Context(), bucketName, itemID, updateData); err != nil {
			// File uploaded but metadata update failed - log but don't fail
			fmt.Printf("Warning: file uploaded but metadata update failed: %v\n", err)
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Upload successful",
			"data": map[string]interface{}{
				"file_url":  fileURL,
				"mime_type": mimeType,
				"size":      fileSize,
			},
		})
	}
}
