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

func TestService_Create(t *testing.T) {
	repo := &mockRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	svc := NewService(repo)

	c, err := svc.Create(context.Background(), CreateInput{
		Name:         "Campaign Promo",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		Status:       "draft",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "Campaign Promo" || c.Status != domain.CampaignStatusDraft {
		t.Fatalf("unexpected created campaign: %+v", c)
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

	newName := "Updated Name"
	newTpl := "tpl-new"
	updated, err := svc.Update(context.Background(), "c1", UpdateInput{
		Name:       &newName,
		TemplateID: &newTpl,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "Updated Name" || updated.TemplateID != "tpl-new" {
		t.Fatalf("unexpected updated campaign: %+v", updated)
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
