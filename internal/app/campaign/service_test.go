package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/agreement"
	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
)

type mockActivityLogRepoForCampaign struct {
	mu        sync.Mutex
	logs      []*domain.ActivityLog
	createErr error
}

func (m *mockActivityLogRepoForCampaign) Create(ctx context.Context, log *domain.ActivityLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockActivityLogRepoForCampaign) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepoForCampaign) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func authContext(userID, role string) context.Context {
	p := &domain.Principal{UserID: userID, Role: role}
	return middleware.ContextWithPrincipal(context.Background(), p)
}

type mockRepo struct {
	campaigns map[string]*domain.NotificationCampaign
	createErr error
	updateErr error
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepo) Create(ctx context.Context, c *domain.NotificationCampaign) error {
	if m.createErr != nil {
		return m.createErr
	}
	if m.campaigns == nil {
		m.campaigns = make(map[string]*domain.NotificationCampaign)
	}
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockRepo) Update(ctx context.Context, c *domain.NotificationCampaign) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.campaigns[c.ID]; ok {
		m.campaigns[c.ID] = c
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	if _, ok := m.campaigns[id]; ok {
		delete(m.campaigns, id)
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if c, ok := m.campaigns[id]; ok {
		c.Status = status
		c.StartedAt = startedAt
		c.CompletedAt = completedAt
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockRepo) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	var res []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		if c.Status == status {
			res = append(res, c)
		}
	}
	return res, nil
}

func (m *mockRepo) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *mockRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	var res []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		if c.Status == domain.CampaignStatusScheduled && c.ScheduledAt != nil && !c.ScheduledAt.After(dueBefore) {
			res = append(res, c)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (m *mockRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		if c.Status == domain.CampaignStatusScheduled {
			c.Status = domain.CampaignStatusRunning
			return c, nil
		}
		return nil, domain.ErrNotFound
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	var res []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		res = append(res, c)
	}
	return res, nil
}

func TestService_List(t *testing.T) {
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {ID: "c1", Name: "Campaign 1", Status: domain.CampaignStatusDraft},
		},
	}
	svc := NewService(repo, nil, nil)

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("unexpected list output: %+v", list)
	}
}

func TestService_GetByID(t *testing.T) {
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {ID: "c1", Name: "Campaign 1", Status: domain.CampaignStatusDraft},
		},
	}
	svc := NewService(repo, nil, nil)

	// 1. Invalid empty ID
	if _, err := svc.GetByID(context.Background(), "  "); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	// 2. Found
	c, err := svc.GetByID(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID != "c1" {
		t.Fatalf("expected c1, got %s", c.ID)
	}

	// 3. Not Found
	if _, err := svc.GetByID(context.Background(), "non-existent"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Create(t *testing.T) {
	repo := &mockRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	svc := NewService(repo, nil, nil)

	// 1. Create draft without scheduledAt
	c1, err := svc.Create(context.Background(), CreateInput{
		Name:         "Campaign Promo",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c1.Name != "Campaign Promo" || c1.Status != domain.CampaignStatusDraft {
		t.Fatalf("unexpected created campaign: %+v", c1)
	}
	if c1.ScheduledAt != nil {
		t.Fatalf("expected nil ScheduledAt, got %v", c1.ScheduledAt)
	}

	// 2. Create campaign with scheduledAt (initial status remains draft)
	futureTime := time.Now().Add(24 * time.Hour).UTC()
	c2, err := svc.Create(context.Background(), CreateInput{
		Name:         "Scheduled Campaign",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		ScheduledAt:  &futureTime,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c2.Status != domain.CampaignStatusDraft || c2.ScheduledAt == nil || !c2.ScheduledAt.Equal(futureTime) {
		t.Fatalf("unexpected campaign with schedule: %+v", c2)
	}

	// 3. Reject creating with non-draft status (e.g. running, completed, scheduled)
	nonDraftStatuses := []string{"running", "scheduled", "completed", "sending", "failed", "cancelled", "invalid"}
	for _, st := range nonDraftStatuses {
		_, err = svc.Create(context.Background(), CreateInput{
			Name:       "Non-draft Campaign",
			TemplateID: "tpl-1",
			Status:     st,
		})
		if err != ErrInvalidInput {
			t.Fatalf("expected ErrInvalidInput when creating campaign with status %q, got %v", st, err)
		}
	}

	// 4. Create with missing name or templateID
	if _, err = svc.Create(context.Background(), CreateInput{TemplateID: "tpl-1"}); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for missing name")
	}
	if _, err = svc.Create(context.Background(), CreateInput{Name: "Name"}); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for missing templateID")
	}
}

func TestService_Update(t *testing.T) {
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:           "c1",
				Name:         "Old Name",
				TemplateID:   "tpl-old",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
			},
		},
	}
	svc := NewService(repo, nil, nil)

	// 1. Update basic info and schedule
	newName := "Updated Name"
	newTpl := "tpl-new"
	schedTime := time.Now().Add(2 * time.Hour).UTC()

	updated, err := svc.Update(context.Background(), "c1", UpdateInput{
		Name:        &newName,
		TemplateID:  &newTpl,
		ScheduledAt: &schedTime,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "Updated Name" || updated.TemplateID != "tpl-new" || updated.Status != domain.CampaignStatusDraft {
		t.Fatalf("unexpected updated campaign: %+v", updated)
	}
	if updated.ScheduledAt == nil || !updated.ScheduledAt.Equal(schedTime) {
		t.Fatalf("expected scheduledAt %v, got %v", schedTime, updated.ScheduledAt)
	}

	// 2. Update with matching existing status (no-op) succeeds
	sameStatus := "draft"
	updatedSame, err := svc.Update(context.Background(), "c1", UpdateInput{
		Status: &sameStatus,
	})
	if err != nil {
		t.Fatalf("unexpected error with same status: %v", err)
	}
	if updatedSame.Status != domain.CampaignStatusDraft {
		t.Fatalf("expected status draft, got %s", updatedSame.Status)
	}

	// 3. Reject client-driven status mutation (e.g. running, completed, cancelled)
	attemptedStatuses := []string{"running", "scheduled", "completed", "cancelled", "invalid"}
	for _, st := range attemptedStatuses {
		val := st
		if _, err := svc.Update(context.Background(), "c1", UpdateInput{Status: &val}); err != ErrInvalidInput {
			t.Fatalf("expected ErrInvalidInput when client attempts to update status to %q, got %v", st, err)
		}
	}
}

func TestService_Delete(t *testing.T) {
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:   "c1",
				Name: "Campaign 1",
			},
		},
	}
	svc := NewService(repo, nil, nil)

	// 1. Invalid empty ID
	if err := svc.Delete(context.Background(), ""); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	// 2. Successful delete
	if err := svc.Delete(context.Background(), "c1"); err != nil {
		t.Fatalf("failed to delete campaign: %v", err)
	}

	// 3. Not found on repeat delete
	if err := svc.Delete(context.Background(), "c1"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Schedule(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)
	// Non-UTC timezone (e.g. UTC+3)
	loc := time.FixedZone("UTC+3", 3*3600)
	futureWithZone := futureTime.In(loc)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"draft-1": {
				ID:           "draft-1",
				Name:         "Draft Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			"sched-1": {
				ID:           "sched-1",
				Name:         "Scheduled Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusScheduled,
				ScheduledAt:  &futureTime,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	svc := NewService(repo, nil, nil)

	// 1. Invalid empty ID
	if _, err := svc.Schedule(context.Background(), "", futureTime); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty ID, got %v", err)
	}

	// 2. Zero scheduledAt is rejected
	if _, err := svc.Schedule(context.Background(), "draft-1", time.Time{}); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for zero time, got %v", err)
	}

	// 3. Past scheduledAt is rejected
	pastTime := now.Add(-10 * time.Minute)
	if _, err := svc.Schedule(context.Background(), "draft-1", pastTime); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for past time, got %v", err)
	}

	// 4. Not found campaign
	if _, err := svc.Schedule(context.Background(), "non-existent", futureTime); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 5. Scheduling an already scheduled campaign is rejected
	if _, err := svc.Schedule(context.Background(), "sched-1", futureTime); err != ErrInvalidState {
		t.Fatalf("expected ErrInvalidState when scheduling already scheduled campaign, got %v", err)
	}

	// 6. Scheduling running/sending/completed/failed/cancelled campaigns is rejected
	invalidStatuses := []domain.CampaignStatus{
		domain.CampaignStatusRunning,
		domain.CampaignStatusSending,
		domain.CampaignStatusCompleted,
		domain.CampaignStatusPartiallyFailed,
		domain.CampaignStatusFailed,
		domain.CampaignStatusCancelled,
	}
	for _, st := range invalidStatuses {
		campID := "camp-" + string(st)
		repo.campaigns[campID] = &domain.NotificationCampaign{
			ID:           campID,
			Name:         "Test Campaign",
			TemplateID:   "tpl-1",
			CampaignType: "broadcast",
			Status:       st,
		}
		if _, err := svc.Schedule(context.Background(), campID, futureTime); err != ErrInvalidState {
			t.Fatalf("expected ErrInvalidState for status %s, got %v", st, err)
		}
	}

	// 7. draft -> scheduled succeeds, scheduledAt persisted and normalized to UTC
	scheduled, err := svc.Schedule(context.Background(), "draft-1", futureWithZone)
	if err != nil {
		t.Fatalf("unexpected error scheduling draft: %v", err)
	}
	if scheduled.Status != domain.CampaignStatusScheduled {
		t.Fatalf("expected status %s, got %s", domain.CampaignStatusScheduled, scheduled.Status)
	}
	if scheduled.ScheduledAt == nil {
		t.Fatal("expected non-nil ScheduledAt")
	}
	if scheduled.ScheduledAt.Location() != time.UTC {
		t.Fatalf("expected UTC location, got %v", scheduled.ScheduledAt.Location())
	}
	if !scheduled.ScheduledAt.Equal(futureTime) {
		t.Fatalf("expected scheduledAt %v, got %v", futureTime, scheduled.ScheduledAt)
	}
}

func TestService_Cancel(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"sched-1": {
				ID:           "sched-1",
				Name:         "Scheduled Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusScheduled,
				ScheduledAt:  &futureTime,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			"draft-1": {
				ID:           "draft-1",
				Name:         "Draft Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	svc := NewService(repo, nil, nil)

	// 1. Invalid empty ID
	if _, err := svc.Cancel(context.Background(), ""); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty ID, got %v", err)
	}

	// 2. Not found campaign
	if _, err := svc.Cancel(context.Background(), "non-existent"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. Cancelling draft campaign is rejected
	if _, err := svc.Cancel(context.Background(), "draft-1"); err != ErrInvalidState {
		t.Fatalf("expected ErrInvalidState when cancelling draft, got %v", err)
	}

	// 4. Cancelling running/sending/completed/failed/already cancelled campaigns is rejected
	invalidStatuses := []domain.CampaignStatus{
		domain.CampaignStatusRunning,
		domain.CampaignStatusSending,
		domain.CampaignStatusCompleted,
		domain.CampaignStatusPartiallyFailed,
		domain.CampaignStatusFailed,
		domain.CampaignStatusCancelled,
	}
	for _, st := range invalidStatuses {
		campID := "camp-" + string(st)
		repo.campaigns[campID] = &domain.NotificationCampaign{
			ID:           campID,
			Name:         "Test Campaign",
			TemplateID:   "tpl-1",
			CampaignType: "broadcast",
			Status:       st,
		}
		if _, err := svc.Cancel(context.Background(), campID); err != ErrInvalidState {
			t.Fatalf("expected ErrInvalidState when cancelling campaign in status %s, got %v", st, err)
		}
	}

	// 5. scheduled -> cancelled succeeds, scheduledAt is preserved
	cancelled, err := svc.Cancel(context.Background(), "sched-1")
	if err != nil {
		t.Fatalf("unexpected error cancelling scheduled campaign: %v", err)
	}
	if cancelled.Status != domain.CampaignStatusCancelled {
		t.Fatalf("expected status %s, got %s", domain.CampaignStatusCancelled, cancelled.Status)
	}
	if cancelled.ScheduledAt == nil || !cancelled.ScheduledAt.Equal(futureTime) {
		t.Fatalf("expected preserved scheduledAt %v, got %v", futureTime, cancelled.ScheduledAt)
	}
}

func TestCampaignService_Audit_Create_Success(t *testing.T) {
	repo := &mockRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-promo"] = &domain.EmailTemplate{
		ID:            "tpl-promo",
		TemplateKey:   "summer_promo",
		Subject:       "Summer Discounts!",
		HTMLBody:      "<h1>Save big!</h1>",
		PlainTextBody: "Save big!",
		Locale:        "en",
		Version:       3,
		Status:        domain.TemplateStatusActive,
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	c, err := svc.Create(ctx, CreateInput{
		Name:         "Summer Campaign",
		TemplateID:   "tpl-promo",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err != nil {
		t.Fatalf("unexpected error creating campaign: %v", err)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignCreated {
		t.Errorf("expected event %s, got %s", domain.EventCampaignCreated, entry.EventType)
	}
	if entry.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type user, got %s", entry.ActorType)
	}
	if entry.ActorUserID == nil || *entry.ActorUserID != "usr-admin-1" {
		t.Errorf("expected actor user ID usr-admin-1, got %v", entry.ActorUserID)
	}
	if entry.TargetType != domain.TargetTypeCampaign {
		t.Errorf("expected target type campaign, got %s", entry.TargetType)
	}
	if entry.TargetID != c.ID {
		t.Errorf("expected target ID %s, got %s", c.ID, entry.TargetID)
	}
	if entry.TargetName == nil || *entry.TargetName != "Summer Campaign" {
		t.Errorf("expected target name 'Summer Campaign', got %v", entry.TargetName)
	}
	if entry.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %s", entry.Status)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["campaignId"] != c.ID {
		t.Errorf("expected campaignId in details, got %v", details["campaignId"])
	}
	if details["campaignName"] != "Summer Campaign" {
		t.Errorf("expected campaignName in details, got %v", details["campaignName"])
	}
	if details["templateId"] != "tpl-promo" {
		t.Errorf("expected templateId in details, got %v", details["templateId"])
	}
	if details["templateKey"] != "summer_promo" {
		t.Errorf("expected templateKey in details, got %v", details["templateKey"])
	}
	if details["templateVersion"] != float64(3) {
		t.Errorf("expected templateVersion 3, got %v", details["templateVersion"])
	}
	if details["subject"] != "Summer Discounts!" {
		t.Errorf("expected subject in details, got %v", details["subject"])
	}
	if details["bodyHtml"] != "<h1>Save big!</h1>" {
		t.Errorf("expected bodyHtml in details, got %v", details["bodyHtml"])
	}
	if details["bodyPlain"] != "Save big!" {
		t.Errorf("expected bodyPlain in details, got %v", details["bodyPlain"])
	}
	if details["locale"] != "en" {
		t.Errorf("expected locale en, got %v", details["locale"])
	}
	if details["initialStatus"] != "draft" {
		t.Errorf("expected initialStatus draft, got %v", details["initialStatus"])
	}
}

func TestCampaignService_Audit_Create_Failed_NoAudit(t *testing.T) {
	repo := &mockRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
		createErr: errors.New("db insert failure"),
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Create(ctx, CreateInput{
		Name:         "Summer Campaign",
		TemplateID:   "tpl-promo",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err == nil {
		t.Fatal("expected error from create, got nil")
	}

	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs on creation failure, got %d", len(auditRepo.logs))
	}
}

func TestCampaignService_Audit_Create_MissingPrincipal_ReturnsError(t *testing.T) {
	repo := &mockRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	// Context without auth principal
	_, err := svc.Create(context.Background(), CreateInput{
		Name:         "Unauth Campaign",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err == nil {
		t.Fatal("expected error due to missing principal, got nil")
	}
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal, got %v", err)
	}
	if len(repo.campaigns) != 0 {
		t.Fatalf("expected no campaign created in repo, got %d", len(repo.campaigns))
	}
	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs, got %d", len(auditRepo.logs))
	}
}

func TestCampaignService_Audit_Create_AuditFailure_PropagatesError(t *testing.T) {
	repo := &mockRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	auditRepo := &mockActivityLogRepoForCampaign{createErr: errors.New("audit db failure")}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Create(ctx, CreateInput{
		Name:         "Campaign Promo",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err == nil {
		t.Fatal("expected audit failure to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "audit db failure") {
		t.Fatalf("expected 'audit db failure' error, got %v", err)
	}
}

func TestCampaignService_Audit_Schedule_Success(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"draft-sched-1": {
				ID:           "draft-sched-1",
				Name:         "Scheduled Promo",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	scheduled, err := svc.Schedule(ctx, "draft-sched-1", futureTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scheduled.Status != domain.CampaignStatusScheduled {
		t.Fatalf("expected scheduled status, got %s", scheduled.Status)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignScheduled {
		t.Errorf("expected event %s, got %s", domain.EventCampaignScheduled, entry.EventType)
	}
	if entry.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type user, got %s", entry.ActorType)
	}
	if entry.ActorUserID == nil || *entry.ActorUserID != "usr-admin-1" {
		t.Errorf("expected actor user ID usr-admin-1, got %v", entry.ActorUserID)
	}
	if entry.TargetType != domain.TargetTypeCampaign {
		t.Errorf("expected target type campaign, got %s", entry.TargetType)
	}
	if entry.TargetID != "draft-sched-1" {
		t.Errorf("expected target ID draft-sched-1, got %s", entry.TargetID)
	}
	if entry.TargetName == nil || *entry.TargetName != "Scheduled Promo" {
		t.Errorf("expected target name 'Scheduled Promo', got %v", entry.TargetName)
	}
	if entry.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %s", entry.Status)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["campaignId"] != "draft-sched-1" {
		t.Errorf("expected campaignId in details, got %v", details["campaignId"])
	}
	if details["previousStatus"] != "draft" {
		t.Errorf("expected previousStatus draft, got %v", details["previousStatus"])
	}
	if details["resultingStatus"] != "scheduled" {
		t.Errorf("expected resultingStatus scheduled, got %v", details["resultingStatus"])
	}
	if details["scheduledAt"] == nil {
		t.Error("expected scheduledAt in details")
	}
}

func TestCampaignService_Audit_Schedule_Failed_NoAudit(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"sched-already": {
				ID:          "sched-already",
				Name:        "Already Sched",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &now,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Schedule(ctx, "sched-already", now.Add(1*time.Hour))
	if err == nil {
		t.Fatal("expected error scheduling already scheduled campaign, got nil")
	}

	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs on scheduling failure, got %d", len(auditRepo.logs))
	}
}

func TestCampaignService_Audit_Schedule_AuditFailure_PropagatesError(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"draft-1": {
				ID:        "draft-1",
				Name:      "Draft",
				Status:    domain.CampaignStatusDraft,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{createErr: errors.New("audit insert failed")}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Schedule(ctx, "draft-1", futureTime)
	if err == nil {
		t.Fatal("expected error when audit fails, got nil")
	}
	if !strings.Contains(err.Error(), "audit insert failed") {
		t.Fatalf("expected audit error, got %v", err)
	}
}

func TestCampaignService_Audit_Cancel_Success(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"sched-cancel-1": {
				ID:           "sched-cancel-1",
				Name:         "Cancelling Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusScheduled,
				ScheduledAt:  &futureTime,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	cancelled, err := svc.Cancel(ctx, "sched-cancel-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cancelled.Status != domain.CampaignStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignCancelled {
		t.Errorf("expected event %s, got %s", domain.EventCampaignCancelled, entry.EventType)
	}
	if entry.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type user, got %s", entry.ActorType)
	}
	if entry.ActorUserID == nil || *entry.ActorUserID != "usr-admin-1" {
		t.Errorf("expected actor user ID usr-admin-1, got %v", entry.ActorUserID)
	}
	if entry.TargetType != domain.TargetTypeCampaign {
		t.Errorf("expected target type campaign, got %s", entry.TargetType)
	}
	if entry.TargetID != "sched-cancel-1" {
		t.Errorf("expected target ID sched-cancel-1, got %s", entry.TargetID)
	}
	if entry.TargetName == nil || *entry.TargetName != "Cancelling Campaign" {
		t.Errorf("expected target name 'Cancelling Campaign', got %v", entry.TargetName)
	}
	if entry.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %s", entry.Status)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["campaignId"] != "sched-cancel-1" {
		t.Errorf("expected campaignId in details, got %v", details["campaignId"])
	}
	if details["previousStatus"] != "scheduled" {
		t.Errorf("expected previousStatus scheduled, got %v", details["previousStatus"])
	}
	if details["resultingStatus"] != "cancelled" {
		t.Errorf("expected resultingStatus cancelled, got %v", details["resultingStatus"])
	}
	if details["cancelledAt"] == nil {
		t.Error("expected cancelledAt in details")
	}
}

func TestCampaignService_Audit_Cancel_Failed_NoAudit(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"draft-cancel": {
				ID:        "draft-cancel",
				Name:      "Draft",
				Status:    domain.CampaignStatusDraft,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Cancel(ctx, "draft-cancel")
	if err == nil {
		t.Fatal("expected error cancelling draft campaign, got nil")
	}

	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs on cancel failure, got %d", len(auditRepo.logs))
	}
}

func TestCampaignService_Audit_Cancel_AuditFailure_PropagatesError(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(2 * time.Hour)
	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"sched-1": {
				ID:          "sched-1",
				Name:        "Scheduled",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &futureTime,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
	auditRepo := &mockActivityLogRepoForCampaign{createErr: errors.New("audit cancel log failed")}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, nil, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Cancel(ctx, "sched-1")
	if err == nil {
		t.Fatal("expected error when audit fails, got nil")
	}
	if !strings.Contains(err.Error(), "audit cancel log failed") {
		t.Fatalf("expected audit error, got %v", err)
	}
}

func TestCampaignService_Audit_AgreementRegression_NoDuplicateCampaignCreated(t *testing.T) {
	templateRepo := newMockTemplateRepo()
	templateRepo.templates["tpl-agree-1"] = &domain.EmailTemplate{
		ID:            "tpl-agree-1",
		TemplateKey:   "agreement_2026",
		Name:          "User Agreement 2026",
		TemplateType:  domain.TemplateTypeUserAgreement,
		Subject:       "Updated Terms",
		HTMLBody:      "<p>Terms</p>",
		PlainTextBody: "Terms",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	campaignRepo := &mockRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	// agreement.Service creates campaign directly via campaignRepo.Create and logs agreement.broadcast_created
	agreementSvc := agreement.NewService(campaignRepo, templateRepo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := agreement.CreateBroadcastInput{
		TemplateID: "tpl-agree-1",
		Name:       "Agreement Broadcast 2026",
	}

	camp, err := agreementSvc.CreateBroadcast(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error creating agreement broadcast: %v", err)
	}
	if camp == nil {
		t.Fatal("expected non-nil created campaign")
	}

	// Verify EXACTLY 1 audit log was created, and it is agreement.broadcast_created (NOT campaign.created)
	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected exactly 1 audit log for agreement broadcast, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventAgreementBroadcastCreated {
		t.Errorf("expected event %s, got %s", domain.EventAgreementBroadcastCreated, entry.EventType)
	}
}
