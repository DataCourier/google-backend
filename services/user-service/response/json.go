package response

import (
	"encoding/json"
	"net/http"
)

// Success represents a successful JSON response
type Success struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error represents an error JSON response
type Error struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Details any    `json:"details,omitempty"`
}

// JSON sends a JSON response
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Success sends a success response
func Ok(w http.ResponseWriter, message string, data any) {
	JSON(w, http.StatusOK, Success{
		Message: message,
		Data:    data,
	})
}

// ErrorResponse sends an error response
func Err(w http.ResponseWriter, status int, err string, message string) {
	JSON(w, status, Error{
		Error:   err,
		Message: message,
	})
}

// BadRequest sends a 400 error
func BadRequest(w http.ResponseWriter, message string) {
	Err(w, http.StatusBadRequest, "bad_request", message)
}

// Unauthorized sends a 401 error
func Unauthorized(w http.ResponseWriter, message string) {
	Err(w, http.StatusUnauthorized, "unauthorized", message)
}

// NotFound sends a 404 error
func NotFound(w http.ResponseWriter, message string) {
	Err(w, http.StatusNotFound, "not_found", message)
}

// InternalError sends a 500 error
func InternalError(w http.ResponseWriter, message string) {
	Err(w, http.StatusInternalServerError, "internal_error", message)
}
