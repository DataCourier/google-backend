package buckets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
)

func RegisterOpenBucketRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	bucket := NewPersonalBucket(client)

	r.Route("/buckets/mine", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/{bucket}", openListHandler(bucket))
		r.Post("/{bucket}", openCreateHandler(bucket))
		r.Post("/{bucket}/batch", openBatchHandler(bucket))
		r.Get("/{bucket}/{id}", openGetHandler(bucket))
		r.Put("/{bucket}/{id}", openUpdateHandler(bucket))
		r.Delete("/{bucket}/{id}", openDeleteHandler(bucket))
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func openListHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	paginationKeys := map[string]bool{"limit": true, "offset": true, "order_by": true, "order_dir": true}

	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		filters := make(map[string]interface{})
		var opts *ListOptions

		for key, values := range r.URL.Query() {
			if len(values) == 0 {
				continue
			}
			if paginationKeys[key] {
				if opts == nil {
					opts = &ListOptions{}
				}
				switch key {
				case "limit":
					if v, err := strconv.Atoi(values[0]); err == nil {
						opts.Limit = v
					}
				case "offset":
					if v, err := strconv.Atoi(values[0]); err == nil {
						opts.Offset = v
					}
				case "order_by":
					opts.OrderBy = values[0]
				case "order_dir":
					if values[0] == "desc" {
						opts.OrderDir = firestore.Desc
					} else {
						opts.OrderDir = firestore.Asc
					}
				}
			} else {
				filters[key] = values[0]
			}
		}

		result, err := bucket.List(r.Context(), bucketName, filters, opts)
		if err != nil {
			if strings.Contains(err.Error(), "unauthorized") {
				respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": result.Data, "total": result.Total})
	}
}

func openCreateHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		id, err := bucket.Create(r.Context(), bucketName, data)
		if err != nil {
			if strings.Contains(err.Error(), "id is required") {
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		data["id"] = id
		respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Created", "data": data})
	}
}

func openGetHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")

		data, err := bucket.Get(r.Context(), bucketName, id)
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

func openUpdateHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")

		var data map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if err := bucket.Update(r.Context(), bucketName, id, data); err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "conflict") {
				status = http.StatusConflict
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Updated", "data": data})
	}
}

func openDeleteHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")
		id := chi.URLParam(r, "id")

		if err := bucket.Delete(r.Context(), bucketName, id); err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "forbidden") {
				status = http.StatusForbidden
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "Deleted"})
	}
}

func openBatchHandler(bucket *PersonalBucketImpl) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucketName := chi.URLParam(r, "bucket")

		var records []map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json array"})
			return
		}

		if len(records) > 100 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("batch size %d exceeds limit of 100", len(records))})
			return
		}

		results, err := bucket.Batch(r.Context(), bucketName, records)
		if err != nil {
			if strings.Contains(err.Error(), "unauthorized") {
				respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			} else {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Batch processed", "data": results})
	}
}
