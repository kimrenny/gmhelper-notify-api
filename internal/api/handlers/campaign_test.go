package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/campaign"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type mockCampaignRepo struct {
	campaigns map[string]*domain.NotificationCampaign
}

func (m *mockCampaignRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockCampaignRepo) Create(ctx context.Context, c *domain.NotificationCampaign) error {
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockCampaignRepo) Update(ctx context.Context, c *domain.NotificationCampaign) error {
	if _, ok := m.campaigns[c.ID]; ok {
		m.campaigns[c.ID] = c
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockCampaignRepo) Delete(ctx context.Context, id string) error {
	if _, ok := m.campaigns[id]; ok {
		delete(m.campaigns, id)
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockCampaignRepo) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	if c, ok := m.campaigns[id]; ok {
		c.Status = status
		c.StartedAt = startedAt
		c.CompletedAt = completedAt
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockCampaignRepo) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	var list []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		if c.Status == status {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *mockCampaignRepo) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *mockCampaignRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	var list []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		list = append(list, c)
	}
	return list, nil
}

func TestCampaignHandler_List(t *testing.T) {
	repo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:           "c1",
				Name:         "Campaign 1",
				TemplateID:   "t1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				ScheduledAt:  time.Now(),
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			},
		},
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res []CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(res) != 1 || res[0].ID != "c1" {
		t.Fatalf("unexpected campaigns response: %+v", res)
	}
}

func TestCampaignHandler_Create(t *testing.T) {
	repo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	body, _ := json.Marshal(CreateCampaignRequest{
		Name:         "New Campaign",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var res CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Name != "New Campaign" {
		t.Fatalf("unexpected campaign name: %s", res.Name)
	}
}

func TestCampaignHandler_Update(t *testing.T) {
	repo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:           "c1",
				Name:         "Initial Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			},
		},
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	updatedName := "Renamed Campaign"
	body, _ := json.Marshal(UpdateCampaignRequest{
		Name: &updatedName,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/c1", bytes.NewReader(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Name != "Renamed Campaign" {
		t.Fatalf("unexpected campaign name: %s", res.Name)
	}
}

func TestCampaignHandler_Delete(t *testing.T) {
	repo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:   "c1",
				Name: "Initial Campaign",
			},
		},
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	// 1. Successful delete -> 204 No Content
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/c1", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 No Content, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Not found delete -> 404 Not Found
	reqNotFound := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/c1", nil)
	reqNotFound.SetPathValue("id", "c1")
	recNotFound := httptest.NewRecorder()

	handler.Delete(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 Not Found, got %d", recNotFound.Code)
	}

	// 3. Empty ID -> 400 Bad Request
	reqEmpty := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/", nil)
	recEmpty := httptest.NewRecorder()

	handler.Delete(recEmpty, reqEmpty)

	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", recEmpty.Code)
	}
}
