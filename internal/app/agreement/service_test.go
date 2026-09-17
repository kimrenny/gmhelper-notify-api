package agreement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *mockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if tpl, ok := m.templates[id]; ok {
		return tpl, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	for _, tpl := range m.templates {
		if tpl.TemplateKey == templateKey {
			return tpl, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Delete(ctx context.Context, id string) error {
	delete(m.templates, id)
	return nil
}

func (m *mockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	res := make([]*domain.EmailTemplate, 0, len(m.templates))
	for _, tpl := range m.templates {
		res = append(res, tpl)
	}
	return res, nil
}

type mockCampaignRepo struct {
	campaigns map[string]*domain.NotificationCampaign
}

func (m *mockCampaignRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockCampaignRepo) Create(ctx context.Context, campaign *domain.NotificationCampaign) error {
	m.campaigns[campaign.ID] = campaign
	return nil
}

func (m *mockCampaignRepo) Update(ctx context.Context, campaign *domain.NotificationCampaign) error {
	m.campaigns[campaign.ID] = campaign
	return nil
}

func (m *mockCampaignRepo) Delete(ctx context.Context, id string) error {
	delete(m.campaigns, id)
	return nil
}

func (m *mockCampaignRepo) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	if c, ok := m.campaigns[id]; ok {
		c.Status = status
		if startedAt != nil {
			c.StartedAt = startedAt
		}
		if completedAt != nil {
			c.CompletedAt = completedAt
		}
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockCampaignRepo) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	res := make([]*domain.NotificationCampaign, 0)
	for _, c := range m.campaigns {
		if c.Status == status {
			res = append(res, c)
		}
	}
	return res, nil
}

func (m *mockCampaignRepo) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *mockCampaignRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *mockCampaignRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		c.Status = domain.CampaignStatusRunning
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockCampaignRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	res := make([]*domain.NotificationCampaign, 0, len(m.campaigns))
	for _, c := range m.campaigns {
		res = append(res, c)
	}
	return res, nil
}

func TestAgreementService_CreateBroadcast_Success(t *testing.T) {
	ctx := context.Background()

	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:          "tpl-active-1",
				TemplateKey: "terms_update",
				Name:        "Terms Update 2026",
				Subject:     "Important Update to Terms",
				HTMLBody:    "<p>New Terms</p>",
				Locale:      "en",
				Status:      domain.TemplateStatusActive,
				Version:     1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo)

	input := CreateBroadcastInput{
		TemplateID: "tpl-active-1",
		Name:       "User Agreement Broadcast - Q3",
	}

	campaign, err := service.CreateBroadcast(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if campaign.ID == "" {
		t.Errorf("expected non-empty campaign ID")
	}
	if campaign.Name != "User Agreement Broadcast - Q3" {
		t.Errorf("expected name 'User Agreement Broadcast - Q3', got '%s'", campaign.Name)
	}
	if campaign.TemplateID != "tpl-active-1" {
		t.Errorf("expected templateId 'tpl-active-1', got '%s'", campaign.TemplateID)
	}
	if campaign.CampaignType != "user_agreement" {
		t.Errorf("expected campaignType 'user_agreement', got '%s'", campaign.CampaignType)
	}
	if campaign.Status != domain.CampaignStatusScheduled {
		t.Errorf("expected status 'scheduled', got '%s'", campaign.Status)
	}
	if campaign.ScheduledAt == nil {
		t.Errorf("expected scheduledAt to be set")
	}
}

func TestAgreementService_CreateBroadcast_DefaultName(t *testing.T) {
	ctx := context.Background()

	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:          "tpl-active-1",
				TemplateKey: "terms_update",
				Name:        "Terms of Service",
				Subject:     "Terms update",
				HTMLBody:    "<p>Terms</p>",
				Locale:      "en",
				Status:      domain.TemplateStatusActive,
				Version:     1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo)

	input := CreateBroadcastInput{
		TemplateID: "tpl-active-1",
	}

	campaign, err := service.CreateBroadcast(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedName := "User Agreement - Terms of Service"
	if campaign.Name != expectedName {
		t.Errorf("expected default name '%s', got '%s'", expectedName, campaign.Name)
	}
}

func TestAgreementService_CreateBroadcast_ValidationErrors(t *testing.T) {
	ctx := context.Background()

	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-draft-1": {
				ID:          "tpl-draft-1",
				TemplateKey: "draft_terms",
				Name:        "Draft Terms",
				Subject:     "Draft",
				HTMLBody:    "<p>Draft</p>",
				Locale:      "en",
				Status:      domain.TemplateStatusDraft,
				Version:     1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo)

	// 1. Missing TemplateID
	_, err := service.CreateBroadcast(ctx, CreateBroadcastInput{TemplateID: "   "})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// 2. Non-existent Template
	_, err = service.CreateBroadcast(ctx, CreateBroadcastInput{TemplateID: "non-existent-id"})
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}

	// 3. Inactive (Draft) Template
	_, err = service.CreateBroadcast(ctx, CreateBroadcastInput{TemplateID: "tpl-draft-1"})
	if !errors.Is(err, ErrTemplateInactive) {
		t.Errorf("expected ErrTemplateInactive, got %v", err)
	}
}

func TestAgreementService_CreateBroadcast_DuplicatePrevention(t *testing.T) {
	ctx := context.Background()

	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:          "tpl-active-1",
				TemplateKey: "terms_update",
				Name:        "Terms of Service",
				Subject:     "Terms update",
				HTMLBody:    "<p>Terms</p>",
				Locale:      "en",
				Status:      domain.TemplateStatusActive,
				Version:     1,
			},
		},
	}

	// Existing running user_agreement campaign
	campaignRepo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"cmp-running-1": {
				ID:           "cmp-running-1",
				Name:         "Existing Running Agreement",
				TemplateID:   "tpl-active-1",
				CampaignType: "user_agreement",
				Status:       domain.CampaignStatusRunning,
			},
		},
	}

	service := NewService(campaignRepo, templateRepo)

	_, err := service.CreateBroadcast(ctx, CreateBroadcastInput{TemplateID: "tpl-active-1"})
	if !errors.Is(err, ErrBroadcastInProgress) {
		t.Errorf("expected ErrBroadcastInProgress, got %v", err)
	}
}
