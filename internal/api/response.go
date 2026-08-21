package api

import (
	"net/http"

	"github.com/gmhelper/notify-api/internal/http/response"
)

type ErrorDetail = response.ErrorDetail
type ErrorResponse = response.ErrorResponse

// JSON sends a JSON response with the provided status code and data payload.
func JSON(w http.ResponseWriter, statusCode int, data any) {
	response.JSON(w, statusCode, data)
}

// Error sends a standardized JSON error response.
func Error(w http.ResponseWriter, statusCode int, errorCode, message string) {
	response.Error(w, statusCode, errorCode, message)
}
