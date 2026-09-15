package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/dashboard"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type mockDashboardRepo struct {
	statsToReturn *domain.DashboardStats
	errToReturn   error
	lastLimit     int
}

func (m *mockDashboardRepo) GetDashboardStats(ctx context.Context, recentLimit int) (*domain.DashboardStats, error) {
	m.lastLimit = recentLimit
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	if m.statsToReturn != nil {
		return m.statsToReturn, nil
	}
	return &domain.DashboardStats{
		RecentCampaigns: make([]*domain.RecentCampaignItem, 0),
	}, nil
}

func TestDashboardHandler_GetStats_Success(t *testing.T) {
	now := time.Now().UTC()
	expected := &domain.DashboardStats{
		Campaigns: domain.DashboardCampaignStats{
			Total:     12,
			Running:   2,
			Completed: 8,
			Failed:    2,
		},
		Templates: domain.DashboardTemplateStats{
			Total:  24,
			Active: 20,
			Draft:  4,
		},
		Deliveries: domain.DashboardDeliveryStats{
			TotalMessages: 1284,
			TotalSent:     1200,
			TotalFailed:   84,
			SuccessRate:   93.46,
		},
		RecentCampaigns: []*domain.RecentCampaignItem{
			{
				ID:           "c-1",
				Name:         "Welcome Email",
				TemplateID:   "t-1",
				TemplateName: "Welcome Template",
				Status:       domain.CampaignStatusCompleted,
				CreatedAt:    now,
			},
		},
	}

	repo := &mockDashboardRepo{statsToReturn: expected}
	service := dashboard.NewService(repo)
	log, _ := logger.NewLogger("error")
	handler := NewDashboardHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats?limit=10", nil)
	rr := httptest.NewRecorder()

	handler.GetStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if repo.lastLimit != 10 {
		t.Errorf("expected limit 10 passed to repository, got %d", repo.lastLimit)
	}

	var response domain.DashboardStats
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if response.Campaigns.Total != 12 {
		t.Errorf("expected 12 campaigns, got %d", response.Campaigns.Total)
	}
	if response.Templates.Total != 24 {
		t.Errorf("expected 24 templates, got %d", response.Templates.Total)
	}
	if response.Deliveries.TotalMessages != 1284 {
		t.Errorf("expected 1284 total messages, got %d", response.Deliveries.TotalMessages)
	}
	if response.Deliveries.SuccessRate != 93.46 {
		t.Errorf("expected 93.46 success rate, got %.2f", response.Deliveries.SuccessRate)
	}
	if len(response.RecentCampaigns) != 1 || response.RecentCampaigns[0].Name != "Welcome Email" {
		t.Errorf("unexpected recent campaigns: %+v", response.RecentCampaigns)
	}
}

func TestDashboardHandler_GetStats_DefaultLimit(t *testing.T) {
	repo := &mockDashboardRepo{}
	service := dashboard.NewService(repo)
	log, _ := logger.NewLogger("error")
	handler := NewDashboardHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	rr := httptest.NewRecorder()

	handler.GetStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	if repo.lastLimit != dashboard.DefaultRecentLimit {
		t.Errorf("expected default limit %d, got %d", dashboard.DefaultRecentLimit, repo.lastLimit)
	}
}

func TestDashboardHandler_GetStats_InternalError(t *testing.T) {
	repo := &mockDashboardRepo{errToReturn: errors.New("database connection failed")}
	service := dashboard.NewService(repo)
	log, _ := logger.NewLogger("error")
	handler := NewDashboardHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	rr := httptest.NewRecorder()

	handler.GetStats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 Internal Server Error, got %d", rr.Code)
	}

	var errBody map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}

	errDetail, ok := errBody["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' object in response, got: %v", errBody)
	}
	if errDetail["code"] != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %v", errDetail["code"])
	}
}
