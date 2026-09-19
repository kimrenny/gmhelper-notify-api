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
	dashboardHandler *handlers.DashboardHandler,
	automationHandler *handlers.AutomationHandler,
	agreementHandler *handlers.AgreementHandler,
	settingsHandler *handlers.SettingsHandler,
	activityHandler *handlers.ActivityHandler,
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

	// Settings endpoints
	if settingsHandler != nil {
		apiV1Mux.HandleFunc("GET /settings", settingsHandler.GetSettings)
		apiV1Mux.HandleFunc("PUT /settings", settingsHandler.UpdateSettings)
	}

	// Agreement endpoints
	if agreementHandler != nil {
		apiV1Mux.HandleFunc("POST /agreements/broadcast", agreementHandler.CreateBroadcast)
		apiV1Mux.HandleFunc("POST /agreements", agreementHandler.CreateBroadcast)
	}

	// Dashboard endpoints
	if dashboardHandler != nil {
		apiV1Mux.HandleFunc("GET /dashboard/stats", dashboardHandler.GetStats)
	}

	// Automation endpoints
	if automationHandler != nil {
		apiV1Mux.HandleFunc("GET /automation/rules", automationHandler.List)
		apiV1Mux.HandleFunc("GET /automation/rules/{id}", automationHandler.GetByID)
		apiV1Mux.HandleFunc("POST /automation/rules", automationHandler.Create)
		apiV1Mux.HandleFunc("PUT /automation/rules/{id}", automationHandler.Update)
		apiV1Mux.HandleFunc("DELETE /automation/rules/{id}", automationHandler.Delete)
	}

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
		apiV1Mux.HandleFunc("POST /campaigns/{id}/schedule", campaignHandler.Schedule)
		apiV1Mux.HandleFunc("POST /campaigns/{id}/cancel", campaignHandler.Cancel)
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

	// Activity History endpoints (Owner-only)
	if activityHandler != nil {
		ownerOnly := middleware.RequireOwnerRole()
		apiV1Mux.Handle("GET /activity", ownerOnly(http.HandlerFunc(activityHandler.List)))
		apiV1Mux.Handle("GET /activity/{id}", ownerOnly(http.HandlerFunc(activityHandler.GetByID)))
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
