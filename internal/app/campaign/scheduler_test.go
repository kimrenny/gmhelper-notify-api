package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

func TestScheduler_ProcessDue(t *testing.T) {
	now := time.Now().UTC()
	pastTime1 := now.Add(-10 * time.Minute)
	pastTime2 := now.Add(-5 * time.Minute)
	futureTime := now.Add(1 * time.Hour)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"due-1": {
				ID:          "due-1",
				Name:        "Due Campaign 1",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &pastTime1,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"due-2": {
				ID:          "due-2",
				Name:        "Due Campaign 2",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &pastTime2,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"future-1": {
				ID:          "future-1",
				Name:        "Future Campaign",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &futureTime,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"draft-1": {
				ID:          "draft-1",
				Name:        "Draft Campaign",
				Status:      domain.CampaignStatusDraft,
				ScheduledAt: &pastTime1,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"cancelled-1": {
				ID:          "cancelled-1",
				Name:        "Cancelled Campaign",
				Status:      domain.CampaignStatusCancelled,
				ScheduledAt: &pastTime1,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"running-1": {
				ID:          "running-1",
				Name:        "Running Campaign",
				Status:      domain.CampaignStatusRunning,
				ScheduledAt: &pastTime1,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}

	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, nil, 50*time.Millisecond, 10, log)
	scheduler.nowFn = func() time.Time { return now }

	claimed, err := scheduler.ProcessDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claimed != 2 {
		t.Fatalf("expected 2 claimed campaigns, got %d", claimed)
	}

	if repo.campaigns["due-1"].Status != domain.CampaignStatusRunning {
		t.Errorf("expected due-1 to be running, got %s", repo.campaigns["due-1"].Status)
	}
	if repo.campaigns["due-2"].Status != domain.CampaignStatusRunning {
		t.Errorf("expected due-2 to be running, got %s", repo.campaigns["due-2"].Status)
	}
	if repo.campaigns["future-1"].Status != domain.CampaignStatusScheduled {
		t.Errorf("expected future-1 to remain scheduled, got %s", repo.campaigns["future-1"].Status)
	}
	if repo.campaigns["draft-1"].Status != domain.CampaignStatusDraft {
		t.Errorf("expected draft-1 to remain draft, got %s", repo.campaigns["draft-1"].Status)
	}
}

type errListDueRepo struct {
	mockRepo
}

func (m *errListDueRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return nil, errors.New("db connection failed")
}

func TestScheduler_ProcessDue_ListError(t *testing.T) {
	repo := &errListDueRepo{}
	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, nil, 50*time.Millisecond, 10, log)

	claimed, err := scheduler.ProcessDue(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if claimed != 0 {
		t.Fatalf("expected 0 claimed, got %d", claimed)
	}
}

type raceClaimRepo struct {
	mockRepo
}

func (m *raceClaimRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return []*domain.NotificationCampaign{
		{ID: "c-race-1", Status: domain.CampaignStatusScheduled},
	}, nil
}

func (m *raceClaimRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	// Simulate race: another instance claimed it in the meantime
	return nil, domain.ErrNotFound
}

func TestScheduler_ProcessDue_RaceCondition(t *testing.T) {
	repo := &raceClaimRepo{}
	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, nil, 50*time.Millisecond, 10, log)

	claimed, err := scheduler.ProcessDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed != 0 {
		t.Fatalf("expected 0 claimed due to race, got %d", claimed)
	}
}

type mockPopulator struct {
	populateFunc func(ctx context.Context, campaignID string) (int, error)
	calledIDs    []string
}

func (m *mockPopulator) PopulateAudience(ctx context.Context, campaignID string) (int, error) {
	m.calledIDs = append(m.calledIDs, campaignID)
	if m.populateFunc != nil {
		return m.populateFunc(ctx, campaignID)
	}
	return 1, nil
}

func TestScheduler_ProcessDue_TriggersPopulateAudienceOnSuccessfulClaim(t *testing.T) {
	now := time.Now().UTC()
	pastTime := now.Add(-5 * time.Minute)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"due-pop-1": {
				ID:          "due-pop-1",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &pastTime,
			},
		},
	}

	pop := &mockPopulator{}
	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, pop, 50*time.Millisecond, 10, log)
	scheduler.nowFn = func() time.Time { return now }

	claimed, err := scheduler.ProcessDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claimed != 1 {
		t.Fatalf("expected 1 claimed, got %d", claimed)
	}

	if len(pop.calledIDs) != 1 || pop.calledIDs[0] != "due-pop-1" {
		t.Fatalf("expected PopulateAudience called for due-pop-1, got %v", pop.calledIDs)
	}
}

func TestScheduler_ProcessDue_LosingRaceDoesNotTriggerPopulateAudience(t *testing.T) {
	repo := &raceClaimRepo{}
	pop := &mockPopulator{}
	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, pop, 50*time.Millisecond, 10, log)

	claimed, err := scheduler.ProcessDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed != 0 {
		t.Fatalf("expected 0 claimed, got %d", claimed)
	}
	if len(pop.calledIDs) != 0 {
		t.Fatalf("expected PopulateAudience not called on race loss, got: %v", pop.calledIDs)
	}
}

func TestScheduler_ProcessDue_PopulatorErrorHandled(t *testing.T) {
	now := time.Now().UTC()
	pastTime := now.Add(-5 * time.Minute)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"due-pop-err": {
				ID:          "due-pop-err",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &pastTime,
			},
		},
	}

	pop := &mockPopulator{
		populateFunc: func(ctx context.Context, campaignID string) (int, error) {
			return 0, errors.New("gmhelper-api unavailable")
		},
	}

	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, pop, 50*time.Millisecond, 10, log)
	scheduler.nowFn = func() time.Time { return now }

	// ProcessDue should still report the claim and not panic/fail
	claimed, err := scheduler.ProcessDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed, got %d", claimed)
	}
}

func TestScheduler_StartAndCancel(t *testing.T) {
	now := time.Now().UTC()
	pastTime := now.Add(-5 * time.Minute)

	repo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"due-1": {
				ID:          "due-1",
				Status:      domain.CampaignStatusScheduled,
				ScheduledAt: &pastTime,
			},
		},
	}

	log, _ := logger.NewLogger("error")
	scheduler := NewScheduler(repo, nil, 20*time.Millisecond, 10, log)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait briefly for at least one run
	time.Sleep(50 * time.Millisecond)

	// Stop scheduler via context cancellation
	cancel()

	select {
	case <-done:
		// Clean exit
	case <-time.After(500 * time.Millisecond):
		t.Fatal("scheduler did not stop after context cancellation")
	}

	if repo.campaigns["due-1"].Status != domain.CampaignStatusRunning {
		t.Fatalf("expected campaign to be claimed during start loop, got %s", repo.campaigns["due-1"].Status)
	}
}
