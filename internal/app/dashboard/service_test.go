package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockDashboardRepo struct {
	statsFunc    func(ctx context.Context, recentLimit int) (*domain.DashboardStats, error)
	lastLimit    int
	statsToYield *domain.DashboardStats
	errToYield   error
}

func (m *mockDashboardRepo) GetDashboardStats(ctx context.Context, recentLimit int) (*domain.DashboardStats, error) {
	m.lastLimit = recentLimit
	if m.statsFunc != nil {
		return m.statsFunc(ctx, recentLimit)
	}
	if m.errToYield != nil {
		return nil, m.errToYield
	}
	if m.statsToYield != nil {
		return m.statsToYield, nil
	}
	return &domain.DashboardStats{}, nil
}

func TestService_GetStats_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	expected := &domain.DashboardStats{
		Campaigns: domain.DashboardCampaignStats{
			Total:     10,
			Running:   2,
			Completed: 6,
			Failed:    2,
		},
		Templates: domain.DashboardTemplateStats{
			Total:  5,
			Active: 4,
			Draft:  1,
		},
		Deliveries: domain.DashboardDeliveryStats{
			TotalMessages: 100,
			TotalSent:     90,
			TotalFailed:   10,
			SuccessRate:   90.0,
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

	repo := &mockDashboardRepo{
		statsToYield: expected,
	}

	svc := NewService(repo)
	stats, err := svc.GetStats(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Campaigns.Total != 10 {
		t.Errorf("expected 10 campaigns, got %d", stats.Campaigns.Total)
	}
	if stats.Deliveries.SuccessRate != 90.0 {
		t.Errorf("expected 90.0 success rate, got %.2f", stats.Deliveries.SuccessRate)
	}
	if len(stats.RecentCampaigns) != 1 {
		t.Fatalf("expected 1 recent campaign, got %d", len(stats.RecentCampaigns))
	}
	if repo.lastLimit != 5 {
		t.Errorf("expected limit 5, got %d", repo.lastLimit)
	}
}

func TestService_GetStats_LimitNormalization(t *testing.T) {
	ctx := context.Background()
	repo := &mockDashboardRepo{}
	svc := NewService(repo)

	// Case 1: limit <= 0 -> defaults to DefaultRecentLimit (5)
	_, err := svc.GetStats(ctx, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != DefaultRecentLimit {
		t.Errorf("expected default limit %d for input 0, got %d", DefaultRecentLimit, repo.lastLimit)
	}

	_, err = svc.GetStats(ctx, -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != DefaultRecentLimit {
		t.Errorf("expected default limit %d for input -10, got %d", DefaultRecentLimit, repo.lastLimit)
	}

	// Case 2: limit > MaxRecentLimit -> capped to MaxRecentLimit (50)
	_, err = svc.GetStats(ctx, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != MaxRecentLimit {
		t.Errorf("expected capped limit %d for input 1000, got %d", MaxRecentLimit, repo.lastLimit)
	}

	// Case 3: valid limit
	_, err = svc.GetStats(ctx, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastLimit != 15 {
		t.Errorf("expected limit 15, got %d", repo.lastLimit)
	}
}

func TestService_GetStats_RepositoryError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("db connection failure")
	repo := &mockDashboardRepo{
		errToYield: expectedErr,
	}

	svc := NewService(repo)
	_, err := svc.GetStats(ctx, 5)
	if err == nil {
		t.Fatal("expected repository error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestService_GetStats_NilRepository(t *testing.T) {
	svc := NewService(nil)
	_, err := svc.GetStats(context.Background(), 5)
	if !errors.Is(err, ErrRepositoryNil) {
		t.Fatalf("expected ErrRepositoryNil, got %v", err)
	}
}
