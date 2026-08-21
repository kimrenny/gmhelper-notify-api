package response

import (
	"encoding/json"
	"net/http"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// JSON sends a JSON response with the provided status code and data payload.
func JSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// Error sends a standardized JSON error response.
func Error(w http.ResponseWriter, statusCode int, errorCode, message string) {
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:    errorCode,
			Message: message,
		},
	}
	JSON(w, statusCode, resp)
}
