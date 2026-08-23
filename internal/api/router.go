package api

import (
	"net/http"

	"github.com/gmhelper/notify-api/internal/api/handlers"
)

// NewRouter constructs and configures the HTTP root router.
func NewRouter(healthHandler *handlers.HealthHandler, templateHandler *handlers.TemplateHandler) http.Handler {
	mux := http.NewServeMux()

	// System & probe endpoints outside versioned API
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /ready", healthHandler.Ready)

	// API v1 prefix handler
	apiV1Mux := http.NewServeMux()

	// Template endpoints
	if templateHandler != nil {
		apiV1Mux.HandleFunc("GET /templates", templateHandler.List)
		apiV1Mux.HandleFunc("GET /templates/{id}", templateHandler.GetByID)
		apiV1Mux.HandleFunc("POST /templates", templateHandler.Create)
		apiV1Mux.HandleFunc("PUT /templates/{id}", templateHandler.Update)
		apiV1Mux.HandleFunc("DELETE /templates/{id}", templateHandler.Delete)
	}

	// Fallback for unhandled /api/v1/ routes
	apiV1Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	})

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiV1Mux))

	// Root fallback for unmapped non-API paths
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			JSON(w, http.StatusOK, map[string]string{
				"service": "gmhelper-notify-api",
				"version": "v1",
			})
			return
		}
		Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	})

	return mux
}
