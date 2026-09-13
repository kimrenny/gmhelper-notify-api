package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/campaign"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type CreateCampaignRequest struct {
	Name         string     `json:"name"`
	TemplateID   string     `json:"templateId"`
	CampaignType string     `json:"campaignType,omitempty"`
	Status       string     `json:"status,omitempty"`
	ScheduledAt  *time.Time `json:"scheduledAt,omitempty"`
}

type CampaignResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	TemplateID   string     `json:"templateId"`
	CampaignType string     `json:"campaignType"`
	Status       string     `json:"status"`
	ScheduledAt  time.Time  `json:"scheduledAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type CampaignHandler struct {
	service *campaign.Service
	logger  logger.Logger
}

func NewCampaignHandler(service *campaign.Service, logger logger.Logger) *CampaignHandler {
	return &CampaignHandler{
		service: service,
		logger:  logger,
	}
}

func (h *CampaignHandler) List(w http.ResponseWriter, r *http.Request) {
	campaigns, err := h.service.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list campaigns", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list campaigns")
		return
	}

	res := make([]CampaignResponse, 0, len(campaigns))
	for _, c := range campaigns {
		res = append(res, toCampaignResponse(c))
	}

	response.JSON(w, http.StatusOK, res)
}

func (h *CampaignHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "campaign id is required")
		return
	}

	c, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "campaign not found")
			return
		}
		if errors.Is(err, campaign.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid campaign id")
			return
		}
		h.logger.Error("failed to get campaign", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve campaign")
		return
	}

	response.JSON(w, http.StatusOK, toCampaignResponse(c))
}

func (h *CampaignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	input := campaign.CreateInput{
		Name:         req.Name,
		TemplateID:   req.TemplateID,
		CampaignType: req.CampaignType,
		Status:       req.Status,
		ScheduledAt:  req.ScheduledAt,
	}

	c, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, campaign.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid")
			return
		}
		h.logger.Error("failed to create campaign", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create campaign")
		return
	}

	response.JSON(w, http.StatusCreated, toCampaignResponse(c))
}

func toCampaignResponse(c *domain.NotificationCampaign) CampaignResponse {
	return CampaignResponse{
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
}
