package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gmhelper/notify-api/internal/app/dashboard"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

// DashboardHandler handles dashboard statistics endpoints.
type DashboardHandler struct {
	service *dashboard.Service
	logger  logger.Logger
}

// NewDashboardHandler constructs a new DashboardHandler.
func NewDashboardHandler(service *dashboard.Service, logger logger.Logger) *DashboardHandler {
	return &DashboardHandler{
		service: service,
		logger:  logger,
	}
}

// GetStats handles GET /api/v1/dashboard/stats.
func (h *DashboardHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	limit := dashboard.DefaultRecentLimit
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	stats, err := h.service.GetStats(r.Context(), limit)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("failed to retrieve dashboard stats", logger.Error(err))
		}
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve dashboard stats")
		return
	}

	response.JSON(w, http.StatusOK, stats)
}
