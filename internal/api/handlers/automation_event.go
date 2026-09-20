package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/automation"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

// AutomationEventRequest represents the incoming JSON payload for internal automation events.
type AutomationEventRequest struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	UserID     string           `json:"userId"`
	User       *userclient.User `json:"user,omitempty"`
	Data       map[string]any   `json:"data,omitempty"`
	OccurredAt *time.Time       `json:"occurredAt"`
}

// AutomationEventHandler handles internal automation event ingestion.
type AutomationEventHandler struct {
	engine *automation.Engine
	logger logger.Logger
}

// NewAutomationEventHandler constructs a new AutomationEventHandler.
func NewAutomationEventHandler(engine *automation.Engine, log logger.Logger) *AutomationEventHandler {
	if log == nil {
		log = logger.NewNop()
	}
	return &AutomationEventHandler{
		engine: engine,
		logger: log,
	}
}

// HandleEvent ingests, validates, and dispatches an incoming automation event to the automation runtime engine.
func (h *AutomationEventHandler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "request body cannot be empty")
		return
	}

	var req AutomationEventRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "request body cannot be empty")
			return
		}
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	if strings.TrimSpace(req.ID) == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "event id is required")
		return
	}

	if strings.TrimSpace(req.Type) == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "event type is required")
		return
	}

	if strings.TrimSpace(req.UserID) == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "user id is required")
		return
	}

	if req.OccurredAt == nil || req.OccurredAt.IsZero() {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "occurredAt is required and must be a valid timestamp")
		return
	}

	if h.engine == nil {
		h.logger.Error("automation engine is not configured on handler")
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "automation engine unavailable")
		return
	}

	event := automation.Event{
		ID:         strings.TrimSpace(req.ID),
		Type:       strings.TrimSpace(req.Type),
		UserID:     strings.TrimSpace(req.UserID),
		User:       req.User,
		Data:       req.Data,
		OccurredAt: *req.OccurredAt,
	}

	result, err := h.engine.HandleEvent(r.Context(), event)
	if err != nil {
		h.logger.Error("failed to process automation event",
			logger.String("eventId", event.ID),
			logger.String("eventType", event.Type),
			logger.Error(err),
		)
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to process automation event")
		return
	}

	response.JSON(w, http.StatusOK, result)
}
