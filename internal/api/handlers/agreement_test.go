package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/agreement"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type agreementHandlerMockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *agreementHandlerMockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if tpl, ok := m.templates[id]; ok {
		return tpl, nil
	}
	return nil, domain.ErrNotFound
}

func (m *agreementHandlerMockTemplateRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}

func (m *agreementHandlerMockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	m.templates[template.ID] = template
	return nil
}

func (m *agreementHandlerMockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	m.templates[template.ID] = template
	return nil
}

func (m *agreementHandlerMockTemplateRepo) Delete(ctx context.Context, id string) error {
	delete(m.templates, id)
	return nil
}

func (m *agreementHandlerMockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type agreementHandlerMockCampaignRepo struct {
	campaigns map[string]*domain.NotificationCampaign
}

func (m *agreementHandlerMockCampaignRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *agreementHandlerMockCampaignRepo) Create(ctx context.Context, campaign *domain.NotificationCampaign) error {
	m.campaigns[campaign.ID] = campaign
	return nil
}

func (m *agreementHandlerMockCampaignRepo) Update(ctx context.Context, campaign *domain.NotificationCampaign) error {
	m.campaigns[campaign.ID] = campaign
	return nil
}

func (m *agreementHandlerMockCampaignRepo) Delete(ctx context.Context, id string) error {
	delete(m.campaigns, id)
	return nil
}

func (m *agreementHandlerMockCampaignRepo) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	return nil
}

func (m *agreementHandlerMockCampaignRepo) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	res := make([]*domain.NotificationCampaign, 0)
	for _, c := range m.campaigns {
		if c.Status == status {
			res = append(res, c)
		}
	}
	return res, nil
}

func (m *agreementHandlerMockCampaignRepo) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *agreementHandlerMockCampaignRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *agreementHandlerMockCampaignRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *agreementHandlerMockCampaignRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func TestAgreementHandler_CreateBroadcast_Success(t *testing.T) {
	log, _ := logger.NewLogger("info")
	tplRepo := &agreementHandlerMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:          "tpl-active-1",
				TemplateKey: "terms_update",
				Name:        "Terms of Service Update",
				Status:      domain.TemplateStatusActive,
			},
		},
	}
	campRepo := &agreementHandlerMockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := agreement.NewService(campRepo, tplRepo)
	handler := NewAgreementHandler(service, log)

	reqBody := `{"templateId":"tpl-active-1","name":"User Agreement Broadcast Q3"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agreements/broadcast", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()

	handler.CreateBroadcast(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var res AgreementBroadcastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.ID == "" {
		t.Errorf("expected non-empty ID")
	}
	if res.Name != "User Agreement Broadcast Q3" {
		t.Errorf("expected name 'User Agreement Broadcast Q3', got '%s'", res.Name)
	}
	if res.TemplateID != "tpl-active-1" {
		t.Errorf("expected templateId 'tpl-active-1', got '%s'", res.TemplateID)
	}
	if res.CampaignType != "user_agreement" {
		t.Errorf("expected campaignType 'user_agreement', got '%s'", res.CampaignType)
	}
	if res.Status != "scheduled" {
		t.Errorf("expected status 'scheduled', got '%s'", res.Status)
	}
}

func TestAgreementHandler_CreateBroadcast_ValidationErrors(t *testing.T) {
	log, _ := logger.NewLogger("info")
	tplRepo := &agreementHandlerMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-draft-1": {
				ID:          "tpl-draft-1",
				TemplateKey: "draft_terms",
				Name:        "Draft Terms",
				Status:      domain.TemplateStatusDraft,
			},
		},
	}
	campRepo := &agreementHandlerMockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := agreement.NewService(campRepo, tplRepo)
	handler := NewAgreementHandler(service, log)

	tests := []struct {
		name         string
		body         string
		expectedCode int
	}{
		{
			name:         "Malformed JSON",
			body:         `{"templateId":}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Unknown fields rejected",
			body:         `{"templateId":"tpl-draft-1","unknownField":"extra"}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Missing templateId",
			body:         `{"templateId":""}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Template Not Found",
			body:         `{"templateId":"non-existent"}`,
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "Template Inactive (Draft)",
			body:         `{"templateId":"tpl-draft-1"}`,
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agreements/broadcast", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateBroadcast(rec, req)

			if rec.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d: %s", tc.expectedCode, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAgreementHandler_CreateBroadcast_Conflict_Duplicate(t *testing.T) {
	log, _ := logger.NewLogger("info")
	tplRepo := &agreementHandlerMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:          "tpl-active-1",
				TemplateKey: "terms_update",
				Name:        "Terms of Service",
				Status:      domain.TemplateStatusActive,
			},
		},
	}
	campRepo := &agreementHandlerMockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"cmp-1": {
				ID:           "cmp-1",
				Name:         "Existing Agreement",
				CampaignType: "user_agreement",
				Status:       domain.CampaignStatusRunning,
			},
		},
	}

	service := agreement.NewService(campRepo, tplRepo)
	handler := NewAgreementHandler(service, log)

	reqBody := `{"templateId":"tpl-active-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agreements/broadcast", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()

	handler.CreateBroadcast(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
	}
}
