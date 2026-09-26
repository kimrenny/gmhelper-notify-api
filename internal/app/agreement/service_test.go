package agreement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
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

func (m *mockTemplateRepo) GetByKeyAndLocale(ctx context.Context, templateKey, locale string) (*domain.EmailTemplate, error) {
	for _, tpl := range m.templates {
		if tpl.TemplateKey == templateKey && (tpl.Locale == locale || locale == "") {
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
				ID:           "tpl-active-1",
				TemplateKey:  "terms_update",
				Name:         "Terms Update 2026",
				TemplateType: domain.TemplateTypeUserAgreement,
				Subject:      "Important Update to Terms",
				HTMLBody:     "<p>New Terms</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
				Version:      1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo, nil)

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
				ID:           "tpl-active-1",
				TemplateKey:  "terms_update",
				Name:         "Terms of Service",
				TemplateType: domain.TemplateTypeUserAgreement,
				Subject:      "Terms update",
				HTMLBody:     "<p>Terms</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
				Version:      1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo, nil)

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
				ID:           "tpl-draft-1",
				TemplateKey:  "draft_terms",
				Name:         "Draft Terms",
				TemplateType: domain.TemplateTypeUserAgreement,
				Subject:      "Draft",
				HTMLBody:     "<p>Draft</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusDraft,
				Version:      1,
			},
		},
	}

	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}

	service := NewService(campaignRepo, templateRepo, nil)

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

	service := NewService(campaignRepo, templateRepo, nil)

	_, err := service.CreateBroadcast(ctx, CreateBroadcastInput{TemplateID: "tpl-active-1"})
	if !errors.Is(err, ErrBroadcastInProgress) {
		t.Errorf("expected ErrBroadcastInProgress, got %v", err)
	}
}

type mockActivityLogRepoForAgreement struct {
	recordedLogs []*domain.ActivityLog
	createErr    error
}

func (m *mockActivityLogRepoForAgreement) Create(ctx context.Context, log *domain.ActivityLog) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.recordedLogs = append(m.recordedLogs, log)
	return nil
}

func (m *mockActivityLogRepoForAgreement) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepoForAgreement) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func authContext(userID, role string) context.Context {
	p := &domain.Principal{UserID: userID, Role: role}
	return middleware.ContextWithPrincipal(context.Background(), p)
}

func TestAgreementService_Audit_CreateBroadcast_Success(t *testing.T) {
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:            "tpl-active-1",
				TemplateKey:   "terms_update_2026",
				Name:          "Terms of Service 2026",
				TemplateType:  domain.TemplateTypeUserAgreement,
				Subject:       "Important: Terms of Service Update",
				HTMLBody:      "<h1>Updated Terms</h1><p>Please review.</p>",
				PlainTextBody: "Updated Terms. Please review.",
				Locale:        "en",
				Status:        domain.TemplateStatusActive,
				Version:       2,
			},
		},
	}
	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	auditRepo := &mockActivityLogRepoForAgreement{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(campaignRepo, templateRepo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := CreateBroadcastInput{
		TemplateID: "tpl-active-1",
		Name:       "User Agreement Broadcast 2026",
	}

	campaign, err := svc.CreateBroadcast(ctx, input)
	if err != nil {
		t.Fatalf("expected broadcast creation success, got: %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAgreementBroadcastCreated {
		t.Errorf("expected event type %q, got %q", domain.EventAgreementBroadcastCreated, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type %q, got %q", domain.ActorTypeUser, log.ActorType)
	}
	if log.ActorUserID == nil || *log.ActorUserID != "usr-owner-1" {
		t.Errorf("expected actor user ID 'usr-owner-1', got %v", log.ActorUserID)
	}
	if log.TargetType != domain.TargetTypeAgreement {
		t.Errorf("expected target type %q, got %q", domain.TargetTypeAgreement, log.TargetType)
	}
	if log.TargetID != campaign.ID {
		t.Errorf("expected target ID %q, got %q", campaign.ID, log.TargetID)
	}
	if log.TargetName == nil || *log.TargetName != "User Agreement Broadcast 2026" {
		t.Errorf("expected target name 'User Agreement Broadcast 2026', got %v", log.TargetName)
	}
	if log.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %q", log.Status)
	}
	if !strings.Contains(log.Summary, "User Agreement Broadcast 2026") {
		t.Errorf("expected summary to contain broadcast name, got %q", log.Summary)
	}

	var details agreementBroadcastDetails
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal audit details: %v", err)
	}
	if details.CampaignID != campaign.ID {
		t.Errorf("expected details campaign ID %q, got %q", campaign.ID, details.CampaignID)
	}
	if details.CampaignName != "User Agreement Broadcast 2026" {
		t.Errorf("expected campaign name, got %q", details.CampaignName)
	}
	if details.TemplateID != "tpl-active-1" {
		t.Errorf("expected templateId 'tpl-active-1', got %q", details.TemplateID)
	}
	if details.TemplateKey != "terms_update_2026" {
		t.Errorf("expected templateKey 'terms_update_2026', got %q", details.TemplateKey)
	}
	if details.TemplateName != "Terms of Service 2026" {
		t.Errorf("expected templateName 'Terms of Service 2026', got %q", details.TemplateName)
	}
	if details.TemplateLocale != "en" {
		t.Errorf("expected templateLocale 'en', got %q", details.TemplateLocale)
	}
	if details.TemplateVersion != 2 {
		t.Errorf("expected templateVersion 2, got %d", details.TemplateVersion)
	}
	if details.Subject != "Important: Terms of Service Update" {
		t.Errorf("expected subject, got %q", details.Subject)
	}
	if details.BodyHTML != "<h1>Updated Terms</h1><p>Please review.</p>" {
		t.Errorf("expected HTML body, got %q", details.BodyHTML)
	}
	if details.CampaignStatus != domain.CampaignStatusScheduled {
		t.Errorf("expected campaign status scheduled, got %q", details.CampaignStatus)
	}
}

func TestAgreementService_Audit_CreateBroadcast_FailedValidation_NoAudit(t *testing.T) {
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{},
	}
	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	auditRepo := &mockActivityLogRepoForAgreement{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(campaignRepo, templateRepo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	_, err := svc.CreateBroadcast(ctx, CreateBroadcastInput{
		TemplateID: "tpl-not-found",
	})
	if err == nil {
		t.Fatal("expected error on non-existent template, got nil")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on failed broadcast, got %d", len(auditRepo.recordedLogs))
	}
}

func TestAgreementService_Audit_CreateBroadcast_MissingPrincipal_ReturnsError(t *testing.T) {
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:           "tpl-active-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeUserAgreement,
			},
		},
	}
	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	auditRepo := &mockActivityLogRepoForAgreement{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(campaignRepo, templateRepo, auditSvc)

	// Unauthenticated context
	_, err := svc.CreateBroadcast(context.Background(), CreateBroadcastInput{
		TemplateID: "tpl-active-1",
	})
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for unauthenticated context, got: %v", err)
	}
}

func TestAgreementService_Audit_CreateBroadcast_AuditFailure_PropagatesError(t *testing.T) {
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:           "tpl-active-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeUserAgreement,
			},
		},
	}
	campaignRepo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	auditRepo := &mockActivityLogRepoForAgreement{
		createErr: errors.New("audit disk full"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(campaignRepo, templateRepo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	_, err := svc.CreateBroadcast(ctx, CreateBroadcastInput{
		TemplateID: "tpl-active-1",
	})
	if err == nil {
		t.Fatal("expected audit error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "audit disk full") {
		t.Errorf("expected audit disk full error message, got: %v", err)
	}
}
