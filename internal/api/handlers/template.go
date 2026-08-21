package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type CreateTemplateRequest struct {
	TemplateKey   string `json:"templateKey"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
	HTMLBody      string `json:"htmlBody"`
	PlainTextBody string `json:"plainTextBody,omitempty"`
	Locale        string `json:"locale,omitempty"`
	Status        string `json:"status,omitempty"`
	Version       int    `json:"version,omitempty"`
}

type UpdateTemplateRequest struct {
	TemplateKey   string `json:"templateKey"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
	HTMLBody      string `json:"htmlBody"`
	PlainTextBody string `json:"plainTextBody,omitempty"`
	Locale        string `json:"locale,omitempty"`
	Status        string `json:"status,omitempty"`
	Version       int    `json:"version,omitempty"`
}

type TemplateResponse struct {
	ID            string    `json:"id"`
	TemplateKey   string    `json:"templateKey"`
	Name          string    `json:"name"`
	Subject       string    `json:"subject"`
	HTMLBody      string    `json:"htmlBody"`
	PlainTextBody string    `json:"plainTextBody,omitempty"`
	Locale        string    `json:"locale"`
	Status        string    `json:"status"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type TemplateHandler struct {
	service *template.Service
	logger  logger.Logger
}

func NewTemplateHandler(service *template.Service, logger logger.Logger) *TemplateHandler {
	return &TemplateHandler{
		service: service,
		logger:  logger,
	}
}

func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.service.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list email templates", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list templates")
		return
	}

	res := make([]TemplateResponse, 0, len(templates))
	for _, t := range templates {
		res = append(res, toTemplateResponse(t))
	}

	response.JSON(w, http.StatusOK, res)
}

func (h *TemplateHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "template id is required")
		return
	}

	tpl, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		if errors.Is(err, template.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid template id")
			return
		}
		h.logger.Error("failed to get email template", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve template")
		return
	}

	response.JSON(w, http.StatusOK, toTemplateResponse(tpl))
}

func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	input := template.CreateInput{
		TemplateKey:   req.TemplateKey,
		Name:          req.Name,
		Subject:       req.Subject,
		HTMLBody:      req.HTMLBody,
		PlainTextBody: req.PlainTextBody,
		Locale:        req.Locale,
		Status:        req.Status,
		Version:       req.Version,
	}

	tpl, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, template.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			response.Error(w, http.StatusConflict, "CONFLICT", "a template with this key, locale and version already exists")
			return
		}
		h.logger.Error("failed to create email template", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create template")
		return
	}

	response.JSON(w, http.StatusCreated, toTemplateResponse(tpl))
}

func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "template id is required")
		return
	}

	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON payload")
		return
	}

	input := template.UpdateInput{
		TemplateKey:   req.TemplateKey,
		Name:          req.Name,
		Subject:       req.Subject,
		HTMLBody:      req.HTMLBody,
		PlainTextBody: req.PlainTextBody,
		Locale:        req.Locale,
		Status:        req.Status,
		Version:       req.Version,
	}

	tpl, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		if errors.Is(err, template.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidEntity) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "required fields are missing or invalid")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			response.Error(w, http.StatusConflict, "CONFLICT", "a template with this key, locale and version already exists")
			return
		}
		h.logger.Error("failed to update email template", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update template")
		return
	}

	response.JSON(w, http.StatusOK, toTemplateResponse(tpl))
}

func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "template id is required")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		if errors.Is(err, template.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid template id")
			return
		}
		h.logger.Error("failed to delete email template", logger.String("id", id), logger.Error(err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete template")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toTemplateResponse(t *domain.EmailTemplate) TemplateResponse {
	return TemplateResponse{
		ID:            t.ID,
		TemplateKey:   t.TemplateKey,
		Name:          t.Name,
		Subject:       t.Subject,
		HTMLBody:      t.HTMLBody,
		PlainTextBody: t.PlainTextBody,
		Locale:        t.Locale,
		Status:        string(t.Status),
		Version:       t.Version,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}
