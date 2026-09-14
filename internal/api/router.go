package api

import (
	"net/http"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/http/middleware"
)

// NewRouter constructs and configures the HTTP root router.
func NewRouter(
	healthHandler *handlers.HealthHandler,
	templateHandler *handlers.TemplateHandler,
	campaignHandler *handlers.CampaignHandler,
	directHandler *handlers.DirectNotificationHandler,
	userHandler *handlers.UserHandler,
	authMiddleware middleware.Middleware,
) http.Handler {
	mux := http.NewServeMux()

	// System & probe endpoints outside versioned API (Always public)
	if healthHandler != nil {
		mux.HandleFunc("GET /health", healthHandler.Health)
		mux.HandleFunc("GET /ready", healthHandler.Ready)
	}

	// API v1 prefix handler
	apiV1Mux := http.NewServeMux()

	// Template endpoints
	if templateHandler != nil {
		apiV1Mux.HandleFunc("GET /templates", templateHandler.List)
		apiV1Mux.HandleFunc("GET /templates/{id}", templateHandler.GetByID)
		apiV1Mux.HandleFunc("POST /templates", templateHandler.Create)
		apiV1Mux.HandleFunc("PUT /templates/{id}", templateHandler.Update)
		apiV1Mux.HandleFunc("DELETE /templates/{id}", templateHandler.Delete)
		apiV1Mux.HandleFunc("POST /templates/{id}/preview", templateHandler.Preview)
	}

	// Campaign endpoints
	if campaignHandler != nil {
		apiV1Mux.HandleFunc("GET /campaigns", campaignHandler.List)
		apiV1Mux.HandleFunc("GET /campaigns/{id}", campaignHandler.GetByID)
		apiV1Mux.HandleFunc("POST /campaigns", campaignHandler.Create)
		apiV1Mux.HandleFunc("PUT /campaigns/{id}", campaignHandler.Update)
		apiV1Mux.HandleFunc("DELETE /campaigns/{id}", campaignHandler.Delete)
	}

	// Direct Notification endpoints
	if directHandler != nil {
		apiV1Mux.HandleFunc("POST /notifications/direct", directHandler.Create)
		apiV1Mux.HandleFunc("GET /notifications/direct/pending", directHandler.ListPending)
		apiV1Mux.HandleFunc("GET /notifications/direct/{id}", directHandler.GetByID)
		apiV1Mux.HandleFunc("POST /notifications/direct/{id}/deliver", directHandler.Deliver)
	}

	// User resolution & search endpoints
	if userHandler != nil {
		apiV1Mux.HandleFunc("GET /users/search", userHandler.Search)
	}

	// Fallback for unhandled /api/v1/ routes
	apiV1Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	})

	var apiV1Handler http.Handler = apiV1Mux
	if authMiddleware != nil {
		apiV1Handler = authMiddleware(apiV1Mux)
	}

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiV1Handler))

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
