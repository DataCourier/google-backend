package buckets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
)

// Pattern to detect Kotlin/Java toString() output: ClassName(field=value)
var serializedObjectPattern = regexp.MustCompile(`^[A-Z][a-zA-Z0-9_]*\([a-zA-Z_]+=`)

func validateNoSerializedObjects(data map[string]interface{}, path string) (string, error) {
	for key, value := range data {
		fieldPath := key
		if path != "" {
			fieldPath = path + "." + key
		}

		switch v := value.(type) {
		case string:
			if serializedObjectPattern.MatchString(v) {
				return fieldPath, fmt.Errorf("field '%s' contains serialized object (got '%s...'). Send proper JSON instead", fieldPath, truncate(v, 50))
			}
		case map[string]interface{}:
			if fp, err := validateNoSerializedObjects(v, fieldPath); err != nil {
				return fp, err
			}
		case []interface{}:
			for i, item := range v {
				itemPath := fmt.Sprintf("%s[%d]", fieldPath, i)
				if str, ok := item.(string); ok {
					if serializedObjectPattern.MatchString(str) {
						return itemPath, fmt.Errorf("field '%s' contains serialized object (got '%s...'). Send proper JSON instead", itemPath, truncate(str, 50))
					}
				}
				if m, ok := item.(map[string]interface{}); ok {
					if fp, err := validateNoSerializedObjects(m, itemPath); err != nil {
						return fp, err
					}
				}
			}
		}
	}
	return "", nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

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
func RegisterOpenBucketRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	bucket := NewPersonalBucket(client)

	loadDiscoveredBuckets()

	r.Route("/buckets/mine", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/{bucket}", openListHandler(bucket))
		r.Post("/{bucket}", openCreateHandler(bucket))
		r.Post("/{bucket}/batch", openBatchHandler(bucket))
		r.Get("/{bucket}/{id}", openGetHandler(bucket))
		r.Put("/{bucket}/{id}", openUpdateHandler(bucket))
		r.Delete("/{bucket}/{id}", openDeleteHandler(bucket))
	})

	// Org bucket routes (open - any bucket name)
	registerOpenOrgRoutes(r, client, authMiddleware)
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
		return
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

		if _, err := validateNoSerializedObjects(data, ""); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "validation_error",
				"message": err.Error(),
			})
			return
		}

		id, err := bucket.Create(r.Context(), bucketName, data)
		if err != nil {
			if strings.Contains(err.Error(), "id is required") {
				respondJSON(w, http.StatusBadRequest, map[string]string{
					"error":   "bad_request",
					"message": err.Error(),
				})
				return
			}
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
			respondJSON(w, errorStatus(err), map[string]string{
				"error":   errorCode(err),
				"message": err.Error(),
			})
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

		if _, err := validateNoSerializedObjects(data, ""); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "validation_error",
				"message": err.Error(),
			})
			return
		}

		if err := bucket.Update(r.Context(), bucketName, id, data); err != nil {
			respondJSON(w, errorStatus(err), map[string]string{
				"error":   errorCode(err),
				"message": err.Error(),
			})
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
			respondJSON(w, errorStatus(err), map[string]string{
				"error":   errorCode(err),
				"message": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Deleted successfully",
		})
	}
}

func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	respondJSON(w, status, data)
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

		if len(records) > 100 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "bad_request",
				"message": "Batch size exceeds limit of 100 records",
			})
			return
		}

		for i, record := range records {
			if _, err := validateNoSerializedObjects(record, ""); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{
					"error":   "validation_error",
					"message": fmt.Sprintf("Record %d: %s", i, err.Error()),
				})
				return
			}
		}

		results, err := bucket.Batch(r.Context(), bucketName, records)
		if err != nil {
			respondJSON(w, errorStatus(err), map[string]string{
				"error":   errorCode(err),
				"message": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Batch processed",
			"data":    results,
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

func errorCode(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "forbidden") {
		return "forbidden"
	}
	if strings.Contains(msg, "not found") {
		return "not_found"
	}
	if strings.Contains(msg, "unauthorized") {
		return "unauthorized"
	}
	return "internal_error"
}
