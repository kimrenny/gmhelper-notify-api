package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gmhelper/notify-api/internal/app/settings"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

// SettingsHandler handles application settings endpoints.
type SettingsHandler struct {
	service *settings.Service
	logger  logger.Logger
}

// NewSettingsHandler constructs a new SettingsHandler.
func NewSettingsHandler(service *settings.Service, logger logger.Logger) *SettingsHandler {
	return &SettingsHandler{
		service: service,
		logger:  logger,
	}
}

// GetSettings handles GET /api/v1/settings.
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	res, err := h.service.GetSettings(r.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Error("failed to retrieve app settings", logger.Error(err))
		}
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve settings")
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// UpdateSettings handles PUT /api/v1/settings.
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input settings.UpdateAppSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON request body")
		return
	}

	res, err := h.service.UpdateSettings(r.Context(), input)
	if err != nil {
		if errors.Is(err, settings.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		if h.logger != nil {
			h.logger.Error("failed to update app settings", logger.Error(err))
		}
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update settings")
		return
	}

	response.JSON(w, http.StatusOK, res)
}
