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

func (m *mockCampaignRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}

func (m *mockCampaignRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		if c.Status == domain.CampaignStatusScheduled {
			c.Status = domain.CampaignStatusRunning
			return c, nil
		}
		return nil, domain.ErrNotFound
	}
	return nil, domain.ErrNotFound
}

func (m *mockCampaignRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	var list []*domain.NotificationCampaign
	for _, c := range m.campaigns {
		list = append(list, c)
	}
	return list, nil
}

func TestCampaignHandler_List(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:           "c1",
				Name:         "Campaign 1",
				TemplateID:   "t1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				ScheduledAt:  &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			"c2": {
				ID:           "c2",
				Name:         "Campaign 2",
				TemplateID:   "t1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusRunning,
				ScheduledAt:  nil,
				CreatedAt:    now,
				UpdatedAt:    now,
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

	if len(res) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(res))
	}
}

func TestCampaignHandler_GetByID(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {
				ID:           "c1",
				Name:         "Scheduled Campaign",
				TemplateID:   "t1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusScheduled,
				ScheduledAt:  &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	// 1. Found
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/c1", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var res CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.ID != "c1" || res.Status != "scheduled" || res.ScheduledAt == nil {
		t.Fatalf("unexpected campaign response: %+v", res)
	}

	// 2. Not Found
	reqNF := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/c-none", nil)
	reqNF.SetPathValue("id", "c-none")
	recNF := httptest.NewRecorder()
	handler.GetByID(recNF, reqNF)

	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recNF.Code)
	}
}

func TestCampaignHandler_Create(t *testing.T) {
	repo := &mockCampaignRepo{
		campaigns: make(map[string]*domain.NotificationCampaign),
	}
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	futureTime := time.Now().Add(48 * time.Hour).UTC()
	body, _ := json.Marshal(CreateCampaignRequest{
		Name:         "Scheduled Launch",
		TemplateID:   "tpl-1",
		CampaignType: "broadcast",
		Status:       "draft",
		ScheduledAt:  &futureTime,
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

	if res.Name != "Scheduled Launch" || res.Status != "draft" || res.ScheduledAt == nil {
		t.Fatalf("unexpected campaign response: %+v", res)
	}

	// Reject non-draft creation status (e.g. running)
	badBody, _ := json.Marshal(CreateCampaignRequest{
		Name:       "Running Campaign",
		TemplateID: "tpl-1",
		Status:     "running",
	})
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", bytes.NewReader(badBody))
	recBad := httptest.NewRecorder()
	handler.Create(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when trying to create campaign with status 'running', got %d", recBad.Code)
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
	schedTime := time.Now().Add(1 * time.Hour).UTC()
	body, _ := json.Marshal(UpdateCampaignRequest{
		Name:        &updatedName,
		ScheduledAt: &schedTime,
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

	if res.Name != "Renamed Campaign" || res.Status != "draft" || res.ScheduledAt == nil {
		t.Fatalf("unexpected campaign response: %+v", res)
	}

	// Reject client attempts to arbitrarily alter status via update (e.g. running)
	badStatus := "running"
	badBody, _ := json.Marshal(UpdateCampaignRequest{
		Status: &badStatus,
	})
	reqBad := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/c1", bytes.NewReader(badBody))
	reqBad.SetPathValue("id", "c1")
	recBad := httptest.NewRecorder()
	handler.Update(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when trying to update status to 'running', got %d", recBad.Code)
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

func TestCampaignHandler_Schedule(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(24 * time.Hour)

	repo := &mockCampaignRepo{
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
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	// 1. Successful schedule
	body, _ := json.Marshal(ScheduleCampaignRequest{
		ScheduledAt: futureTime,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/draft-1/schedule", bytes.NewReader(body))
	req.SetPathValue("id", "draft-1")
	rec := httptest.NewRecorder()
	handler.Schedule(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Status != "scheduled" || res.ScheduledAt == nil {
		t.Fatalf("expected scheduled status, got %+v", res)
	}

	// 2. Malformed JSON
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/draft-1/schedule", bytes.NewReader([]byte("invalid json")))
	reqBad.SetPathValue("id", "draft-1")
	recBad := httptest.NewRecorder()
	handler.Schedule(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on malformed JSON, got %d", recBad.Code)
	}

	// 3. Past scheduledAt
	pastBody, _ := json.Marshal(ScheduleCampaignRequest{
		ScheduledAt: now.Add(-1 * time.Hour),
	})
	reqPast := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/draft-1/schedule", bytes.NewReader(pastBody))
	reqPast.SetPathValue("id", "draft-1")
	recPast := httptest.NewRecorder()
	handler.Schedule(recPast, reqPast)
	if recPast.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on past scheduledAt, got %d", recPast.Code)
	}

	// 4. Invalid state (already scheduled)
	reqAlready := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/sched-1/schedule", bytes.NewReader(body))
	reqAlready.SetPathValue("id", "sched-1")
	recAlready := httptest.NewRecorder()
	handler.Schedule(recAlready, reqAlready)
	if recAlready.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on already scheduled campaign, got %d", recAlready.Code)
	}

	// 5. Not found
	reqNF := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/c-none/schedule", bytes.NewReader(body))
	reqNF.SetPathValue("id", "c-none")
	recNF := httptest.NewRecorder()
	handler.Schedule(recNF, reqNF)
	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", recNF.Code)
	}

	// 6. Empty ID
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns//schedule", bytes.NewReader(body))
	recEmpty := httptest.NewRecorder()
	handler.Schedule(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on empty ID, got %d", recEmpty.Code)
	}
}

func TestCampaignHandler_Cancel(t *testing.T) {
	now := time.Now().UTC()
	futureTime := now.Add(24 * time.Hour)

	repo := &mockCampaignRepo{
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
	log, _ := logger.NewLogger("error")
	service := campaign.NewService(repo)
	handler := NewCampaignHandler(service, log)

	// 1. Successful cancel
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/sched-1/cancel", nil)
	req.SetPathValue("id", "sched-1")
	rec := httptest.NewRecorder()
	handler.Cancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res CampaignResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Status != "cancelled" || res.ScheduledAt == nil {
		t.Fatalf("expected cancelled status with preserved scheduledAt, got %+v", res)
	}

	// 2. Invalid state (cancelling draft)
	reqDraft := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/draft-1/cancel", nil)
	reqDraft.SetPathValue("id", "draft-1")
	recDraft := httptest.NewRecorder()
	handler.Cancel(recDraft, reqDraft)
	if recDraft.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on cancelling draft campaign, got %d", recDraft.Code)
	}

	// 3. Not found
	reqNF := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/c-none/cancel", nil)
	reqNF.SetPathValue("id", "c-none")
	recNF := httptest.NewRecorder()
	handler.Cancel(recNF, reqNF)
	if recNF.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", recNF.Code)
	}

	// 4. Empty ID
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns//cancel", nil)
	recEmpty := httptest.NewRecorder()
	handler.Cancel(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on empty ID, got %d", recEmpty.Code)
	}
}
