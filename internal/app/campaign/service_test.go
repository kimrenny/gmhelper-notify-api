package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockRepo struct {
	campaigns map[string]*domain.NotificationCampaign
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepo) Create(ctx context.Context, c *domain.NotificationCampaign) error {
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockRepo) Update(ctx context.Context, c *domain.NotificationCampaign) error {
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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
	svc := NewService(repo)

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
