package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type CreateDirectNotificationRequest struct {
	TemplateID       string         `json:"templateId"`
	ExternalUserID   string         `json:"externalUserId,omitempty"`
	RecipientEmail   string         `json:"recipientEmail"`
	RecipientName    string         `json:"recipientName,omitempty"`
	NotificationType string         `json:"notificationType,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
}

type DirectNotificationResponse struct {
	ID               string          `json:"id"`
	TemplateID       string          `json:"templateId"`
	ExternalUserID   string          `json:"externalUserId,omitempty"`
	RecipientEmail   string          `json:"recipientEmail"`
	RecipientName    string          `json:"recipientName,omitempty"`
	NotificationType string          `json:"notificationType"`
	DeliveryStatus   string          `json:"deliveryStatus"`
	AttemptsCount    int             `json:"attemptsCount"`
	LastAttemptAt    *time.Time      `json:"lastAttemptAt,omitempty"`
	SentAt           *time.Time      `json:"sentAt,omitempty"`
	ErrorMessage     string          `json:"errorMessage,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type DirectNotificationHandler struct {
	directService   *direct.Service
	deliveryService *direct.DeliveryService
	logger          logger.Logger
}

func NewDirectNotificationHandler(
	directService *direct.Service,
	deliveryService *direct.DeliveryService,
	logger logger.Logger,
) *DirectNotificationHandler {
	return &DirectNotificationHandler{
		directService:   directService,
		deliveryService: deliveryService,
		logger:          logger,
	}
}

func (h *DirectNotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "request body cannot be empty")
		return
	}

	var req CreateDirectNotificationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "request body cannot be empty")
			return
		}
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	externalUserID := req.ExternalUserID
	if principal, ok := middleware.GetPrincipal(r.Context()); ok && principal.UserID != "" {
		externalUserID = principal.UserID
	}

	input := direct.CreateInput{
		TemplateID:       req.TemplateID,
		ExternalUserID:   externalUserID,
		RecipientEmail:   req.RecipientEmail,
		RecipientName:    req.RecipientName,
		NotificationType: domain.NotificationType(req.NotificationType),
		Payload:          req.Payload,
	}

	res, err := h.directService.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		if errors.Is(err, direct.ErrTemplateInactive) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "template is not active for delivery")
			return
		}
		if errors.Is(err, direct.ErrMissingVariable) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		if errors.Is(err, direct.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		h.logger.Error("failed to create direct notification", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create direct notification")
		return
	}

	response.JSON(w, http.StatusCreated, toDirectNotificationResponse(res.Notification))
}

func (h *DirectNotificationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "notification id is required")
		return
	}

	notification, err := h.directService.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "direct notification not found")
			return
		}
		if errors.Is(err, direct.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid notification id")
			return
		}
		h.logger.Error("failed to get direct notification", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve direct notification")
		return
	}

	response.JSON(w, http.StatusOK, toDirectNotificationResponse(notification))
}

func (h *DirectNotificationHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.directService.ListPending(r.Context())
	if err != nil {
		h.logger.Error("failed to list pending direct notifications", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list pending notifications")
		return
	}

	res := make([]DirectNotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		res = append(res, toDirectNotificationResponse(n))
	}

	response.JSON(w, http.StatusOK, res)
}

func (h *DirectNotificationHandler) Deliver(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "notification id is required")
		return
	}

	if err := h.deliveryService.Deliver(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "direct notification not found")
			return
		}
		if errors.Is(err, direct.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid notification id")
			return
		}
		if errors.Is(err, direct.ErrAlreadySent) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "notification is already sent")
			return
		}
		if errors.Is(err, direct.ErrNotificationCancelled) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "notification is cancelled")
			return
		}
		if errors.Is(err, direct.ErrInvalidDeliveryState) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		if errors.Is(err, direct.ErrTemplateInactive) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "referenced template is not active")
			return
		}
		if errors.Is(err, direct.ErrMissingVariable) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		h.logger.Error("failed to deliver direct notification", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "DELIVERY_FAILED", "failed to deliver notification: "+err.Error())
		return
	}

	updated, err := h.directService.GetByID(r.Context(), id)
	if err != nil {
		response.JSON(w, http.StatusOK, map[string]string{
			"id":     id,
			"status": string(domain.DeliveryStatusSent),
		})
		return
	}

	response.JSON(w, http.StatusOK, toDirectNotificationResponse(updated))
}

func toDirectNotificationResponse(n *domain.DirectNotification) DirectNotificationResponse {
	return DirectNotificationResponse{
		ID:               n.ID,
		TemplateID:       n.TemplateID,
		ExternalUserID:   n.ExternalUserID,
		RecipientEmail:   n.RecipientEmail,
		RecipientName:    n.RecipientName,
		NotificationType: string(n.NotificationType),
		DeliveryStatus:   string(n.DeliveryStatus),
		AttemptsCount:    n.AttemptsCount,
		LastAttemptAt:    n.LastAttemptAt,
		SentAt:           n.SentAt,
		ErrorMessage:     n.ErrorMessage,
		Payload:          n.Payload,
		CreatedAt:        n.CreatedAt,
		UpdatedAt:        n.UpdatedAt,
	}
}
