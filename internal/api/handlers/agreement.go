package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gmhelper/notify-api/internal/app/agreement"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type CreateAgreementBroadcastRequest struct {
	TemplateID string `json:"templateId"`
	Name       string `json:"name,omitempty"`
}

type AgreementBroadcastResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	TemplateID   string     `json:"templateId"`
	CampaignType string     `json:"campaignType"`
	Status       string     `json:"status"`
	ScheduledAt  *time.Time `json:"scheduledAt,omitempty"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type AgreementHandler struct {
	service *agreement.Service
	logger  logger.Logger
}

func NewAgreementHandler(service *agreement.Service, logger logger.Logger) *AgreementHandler {
	return &AgreementHandler{
		service: service,
		logger:  logger,
	}
}

// CreateBroadcast handles POST /api/v1/agreements/broadcast.
func (h *AgreementHandler) CreateBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "request body cannot be empty")
		return
	}

	var req CreateAgreementBroadcastRequest
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

	input := agreement.CreateBroadcastInput{
		TemplateID: req.TemplateID,
		Name:       req.Name,
	}

	c, err := h.service.CreateBroadcast(r.Context(), input)
	if err != nil {
		if errors.Is(err, agreement.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid: templateId is required")
			return
		}
		if errors.Is(err, agreement.ErrTemplateNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "referenced email template not found")
			return
		}
		if errors.Is(err, agreement.ErrTemplateInactive) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "referenced template is not active for broadcast")
			return
		}
		if errors.Is(err, agreement.ErrBroadcastInProgress) {
			response.Error(w, http.StatusConflict, "CONFLICT", "a user agreement broadcast is already in progress")
			return
		}

		h.logger.Error("failed to create agreement broadcast", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user agreement broadcast")
		return
	}

	res := AgreementBroadcastResponse{
		ID:           c.ID,
		Name:         c.Name,
		TemplateID:   c.TemplateID,
		CampaignType: c.CampaignType,
		Status:       string(c.Status),
		ScheduledAt:  c.ScheduledAt,
		StartedAt:    c.StartedAt,
		CompletedAt:  c.CompletedAt,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}

	response.JSON(w, http.StatusCreated, res)
}
