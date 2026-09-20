package automation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

func TestScheduler_EvaluateDue(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.AddDate(0, 0, -45)

	rule := createTestRule("rule-sched-1", "Scheduler Test Rule", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldLastActivityAt,
					Operator: domain.OperatorOlderThan,
					Value:    30,
					Unit:     domain.UnitDays,
				},
			},
		},
	}, nil)

	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{rule}}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Subject",
				HTMLBody:     "<p>Body</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	userLister := &mockUserLister{
		pages: [][]*userclient.User{
			{
				{ID: "user-1", Email: "inactive@example.com", Username: "User1", LastActivityAt: &inactiveTime, IsActive: true},
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())
	sched := NewScheduler(engine, userLister, time.Hour, 50, logger.NewNop())

	summary, err := sched.EvaluateDue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error during EvaluateDue: %v", err)
	}

	if summary.TotalUsersEvaluated != 1 {
		t.Errorf("expected 1 user evaluated, got %d", summary.TotalUsersEvaluated)
	}
	if summary.ExecutedCount != 1 {
		t.Errorf("expected 1 executed, got %d", summary.ExecutedCount)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected 1 notification created, got %d", len(directRepo.created))
	}
}

func TestScheduler_NoOverlappingRuns(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())
	sched := NewScheduler(engine, &mockUserLister{}, time.Hour, 50, logger.NewNop())

	// Artificially mark running
	sched.mu.Lock()
	sched.running = true
	sched.mu.Unlock()

	// Verify IsRunning returns true
	if !sched.IsRunning() {
		t.Errorf("expected IsRunning to return true")
	}

	// Reset running
	sched.mu.Lock()
	sched.running = false
	sched.mu.Unlock()
}

func TestScheduler_StartAndStop(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())
	sched := NewScheduler(engine, &mockUserLister{}, 10*time.Millisecond, 50, logger.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		sched.Start(ctx)
	}()

	// Let it run a couple ticks
	time.Sleep(50 * time.Millisecond)

	// Stop it via cancel
	cancel()

	// Wait for goroutine to finish
	wg.Wait()

	if sched.IsRunning() {
		t.Errorf("expected scheduler to not be running after context cancellation")
	}
}
