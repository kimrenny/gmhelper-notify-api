package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/automation"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type CreateAutomationRuleRequest struct {
	Name       string                      `json:"name"`
	TemplateID string                      `json:"templateId"`
	Enabled    *bool                       `json:"enabled,omitempty"`
	Config     domain.AutomationRuleConfig `json:"config"`
}

type UpdateAutomationRuleRequest struct {
	Name       *string                      `json:"name,omitempty"`
	TemplateID *string                      `json:"templateId,omitempty"`
	Enabled    *bool                        `json:"enabled,omitempty"`
	Config     *domain.AutomationRuleConfig `json:"config,omitempty"`
}

type AutomationRuleResponse struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	TemplateID       string                      `json:"templateId"`
	Enabled          bool                        `json:"enabled"`
	Config           domain.AutomationRuleConfig `json:"config"`
	LastEvaluatedAt  *time.Time                  `json:"lastEvaluatedAt,omitempty"`
	NextEvaluationAt *time.Time                  `json:"nextEvaluationAt,omitempty"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
}

func toAutomationRuleResponse(r *domain.AutomationRule) AutomationRuleResponse {
	return AutomationRuleResponse{
		ID:               r.ID,
		Name:             r.Name,
		TemplateID:       r.TemplateID,
		Enabled:          r.Enabled,
		Config:           r.Config,
		LastEvaluatedAt:  r.LastEvaluatedAt,
		NextEvaluationAt: r.NextEvaluationAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

type AutomationHandler struct {
	service *automation.Service
	logger  logger.Logger
}

func NewAutomationHandler(service *automation.Service, logger logger.Logger) *AutomationHandler {
	return &AutomationHandler{
		service: service,
		logger:  logger,
	}
}

func (h *AutomationHandler) List(w http.ResponseWriter, r *http.Request) {
	rules, err := h.service.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list automation rules", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list automation rules")
		return
	}

	res := make([]AutomationRuleResponse, 0, len(rules))
	for _, rule := range rules {
		res = append(res, toAutomationRuleResponse(rule))
	}

	response.JSON(w, http.StatusOK, res)
}

func (h *AutomationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "automation rule id is required")
		return
	}

	rule, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "automation rule not found")
			return
		}
		if errors.Is(err, automation.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid automation rule id")
			return
		}
		h.logger.Error("failed to get automation rule", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve automation rule")
		return
	}

	response.JSON(w, http.StatusOK, toAutomationRuleResponse(rule))
}

func (h *AutomationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	input := automation.CreateInput{
		Name:       req.Name,
		TemplateID: req.TemplateID,
		Enabled:    req.Enabled,
		Config:     req.Config,
	}

	rule, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, automation.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid")
			return
		}
		if errors.Is(err, automation.ErrTemplateNotFound) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "referenced template does not exist")
			return
		}
		h.logger.Error("failed to create automation rule", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create automation rule")
		return
	}

	response.JSON(w, http.StatusCreated, toAutomationRuleResponse(rule))
}

func (h *AutomationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "automation rule id is required")
		return
	}

	var req UpdateAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	input := automation.UpdateInput{
		Name:       req.Name,
		TemplateID: req.TemplateID,
		Enabled:    req.Enabled,
		Config:     req.Config,
	}

	rule, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "automation rule not found")
			return
		}
		if errors.Is(err, automation.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid")
			return
		}
		if errors.Is(err, automation.ErrTemplateNotFound) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "referenced template does not exist")
			return
		}
		h.logger.Error("failed to update automation rule", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update automation rule")
		return
	}

	response.JSON(w, http.StatusOK, toAutomationRuleResponse(rule))
}

func (h *AutomationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "automation rule id is required")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "automation rule not found")
			return
		}
		if errors.Is(err, automation.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid automation rule id")
			return
		}
		h.logger.Error("failed to delete automation rule", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete automation rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
