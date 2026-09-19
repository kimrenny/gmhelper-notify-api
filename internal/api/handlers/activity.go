package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/google/uuid"
)

type ActivityActorResponse struct {
	Type   string  `json:"type"`
	UserID *string `json:"userId,omitempty"`
	Name   *string `json:"name,omitempty"`
	Role   *string `json:"role,omitempty"`
}

type ActivityTargetResponse struct {
	Type string  `json:"type"`
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type ActivityListItemResponse struct {
	ID        string                 `json:"id"`
	EventType string                 `json:"eventType"`
	Actor     ActivityActorResponse  `json:"actor"`
	Target    ActivityTargetResponse `json:"target"`
	Status    string                 `json:"status"`
	Summary   string                 `json:"summary"`
	CreatedAt time.Time              `json:"createdAt"`
}

type ActivityDetailResponse struct {
	ID           string                 `json:"id"`
	EventType    string                 `json:"eventType"`
	Actor        ActivityActorResponse  `json:"actor"`
	Target       ActivityTargetResponse `json:"target"`
	Status       string                 `json:"status"`
	Summary      string                 `json:"summary"`
	Details      json.RawMessage        `json:"details"`
	ErrorMessage *string                `json:"errorMessage,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
}

type ActivityListResponse struct {
	Items  []ActivityListItemResponse `json:"items"`
	Total  int                        `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

func toActivityListItemResponse(log *domain.ActivityLog) ActivityListItemResponse {
	return ActivityListItemResponse{
		ID:        log.ID,
		EventType: log.EventType,
		Actor: ActivityActorResponse{
			Type:   string(log.ActorType),
			UserID: log.ActorUserID,
			Name:   log.ActorName,
			Role:   log.ActorRole,
		},
		Target: ActivityTargetResponse{
			Type: string(log.TargetType),
			ID:   log.TargetID,
			Name: log.TargetName,
		},
		Status:    string(log.Status),
		Summary:   log.Summary,
		CreatedAt: log.CreatedAt,
	}
}

func toActivityDetailResponse(log *domain.ActivityLog) ActivityDetailResponse {
	details := log.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}

	return ActivityDetailResponse{
		ID:        log.ID,
		EventType: log.EventType,
		Actor: ActivityActorResponse{
			Type:   string(log.ActorType),
			UserID: log.ActorUserID,
			Name:   log.ActorName,
			Role:   log.ActorRole,
		},
		Target: ActivityTargetResponse{
			Type: string(log.TargetType),
			ID:   log.TargetID,
			Name: log.TargetName,
		},
		Status:       string(log.Status),
		Summary:      log.Summary,
		Details:      details,
		ErrorMessage: log.ErrorMessage,
		CreatedAt:    log.CreatedAt,
	}
}

type ActivityHandler struct {
	service *audit.Service
	logger  logger.Logger
}

func NewActivityHandler(service *audit.Service, logger logger.Logger) *ActivityHandler {
	return &ActivityHandler{
		service: service,
		logger:  logger,
	}
}

func (h *ActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 20
	if limitStr := strings.TrimSpace(q.Get("limit")); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l <= 0 || l > 100 {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be a positive integer between 1 and 100")
			return
		}
		limit = l
	}

	offset := 0
	if offsetStr := strings.TrimSpace(q.Get("offset")); offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "offset must be a non-negative integer")
			return
		}
		offset = o
	}

	filter := domain.ActivityLogFilter{
		Limit:  limit,
		Offset: offset,
	}

	if eventType := strings.TrimSpace(q.Get("eventType")); eventType != "" {
		filter.EventType = &eventType
	}

	if actorUserID := strings.TrimSpace(q.Get("actorUserId")); actorUserID != "" {
		filter.ActorUserID = &actorUserID
	}

	if targetTypeStr := strings.TrimSpace(q.Get("targetType")); targetTypeStr != "" {
		targetType := domain.ActivityTargetType(targetTypeStr)
		if !targetType.IsValid() {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid or unsupported targetType")
			return
		}
		filter.TargetType = &targetType
	}

	if targetID := strings.TrimSpace(q.Get("targetId")); targetID != "" {
		filter.TargetID = &targetID
	}

	if statusStr := strings.TrimSpace(q.Get("status")); statusStr != "" {
		status := domain.ActivityStatus(statusStr)
		if !status.IsValid() {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid or unsupported status")
			return
		}
		filter.Status = &status
	}

	if fromDateStr := strings.TrimSpace(q.Get("fromDate")); fromDateStr != "" {
		t, err := parseDateTime(fromDateStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid fromDate format, expected RFC3339 or YYYY-MM-DD")
			return
		}
		filter.FromDate = &t
	}

	if toDateStr := strings.TrimSpace(q.Get("toDate")); toDateStr != "" {
		t, err := parseDateTime(toDateStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid toDate format, expected RFC3339 or YYYY-MM-DD")
			return
		}
		filter.ToDate = &t
	}

	logs, total, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list activity logs", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list activity logs")
		return
	}

	items := make([]ActivityListItemResponse, 0, len(logs))
	for _, l := range logs {
		items = append(items, toActivityListItemResponse(l))
	}

	response.JSON(w, http.StatusOK, ActivityListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *ActivityHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "activity log id is required")
		return
	}

	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid activity log id, expected valid UUID")
		return
	}

	log, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "activity log not found")
			return
		}
		if errors.Is(err, audit.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid activity log id")
			return
		}
		h.logger.Error("failed to get activity log", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve activity log")
		return
	}

	response.JSON(w, http.StatusOK, toActivityDetailResponse(log))
}

func parseDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid date format")
}
