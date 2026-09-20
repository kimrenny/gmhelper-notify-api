package automation

import (
	"context"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

func TestEngine_E2E_FullScenarios(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	auditRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(auditRepo)
	log := logger.NewNop()

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditSvc, log)

	// Setup template
	tpl := &domain.EmailTemplate{
		ID:            "tpl-e2e-1",
		TemplateKey:   "tpl_key_welcome",
		Name:          "E2E Template",
		TemplateType:  domain.TemplateTypeAutomation,
		Status:        domain.TemplateStatusActive,
		Subject:       "Welcome {{username}}!",
		HTMLBody:      "<p>Welcome {{username}}</p>",
		PlainTextBody: "Welcome {{username}}",
	}
	templateRepo.templates["tpl-e2e-1"] = tpl

	cooldownDays := 7
	rule := createTestRule("rule-e2e-1", "Active Welcome", "tpl-e2e-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldIsActive,
					Operator: domain.OperatorEquals,
					Value:    true,
				},
			},
		},
	}, &cooldownDays)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	ctx := context.Background()

	// -------------------------------------------------------------
	// TEST A: Successful execution
	// -------------------------------------------------------------
	t.Run("TestA_SuccessfulExecution", func(t *testing.T) {
		eventA := Event{
			ID:         "evt-001",
			Type:       EventUserRegistered,
			UserID:     "user-1",
			User:       &userclient.User{ID: "user-1", Email: "alex@example.com", Username: "Alex", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}

		res, err := engine.HandleEvent(ctx, eventA)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 1 || res.Results[0].Status != StatusExecuted {
			t.Fatalf("expected StatusExecuted, got %+v", res.Results)
		}
		if res.Results[0].NotificationID == nil {
			t.Fatalf("expected non-nil notification ID")
		}
		if len(directRepo.created) != 1 {
			t.Fatalf("expected 1 direct notification, got %d", len(directRepo.created))
		}
		if directRepo.created[0].NotificationType != domain.NotificationTypeAutomation {
			t.Errorf("expected NotificationTypeAutomation, got %s", directRepo.created[0].NotificationType)
		}
		if directRepo.created[0].DeliveryStatus != domain.DeliveryStatusPending {
			t.Errorf("expected DeliveryStatusPending, got %s", directRepo.created[0].DeliveryStatus)
		}

		// Verify audit event
		if len(auditRepo.records) != 1 {
			t.Fatalf("expected 1 audit log record, got %d", len(auditRepo.records))
		}
		if auditRepo.records[0].EventType != domain.EventAutomationExecuted {
			t.Errorf("expected %s, got %s", domain.EventAutomationExecuted, auditRepo.records[0].EventType)
		}
	})

	// -------------------------------------------------------------
	// TEST B: Event Idempotency (Send exact same event ID again)
	// -------------------------------------------------------------
	t.Run("TestB_EventIdempotency", func(t *testing.T) {
		eventA := Event{
			ID:         "evt-001",
			Type:       EventUserRegistered,
			UserID:     "user-1",
			User:       &userclient.User{ID: "user-1", Email: "alex@example.com", Username: "Alex", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}

		res, err := engine.HandleEvent(ctx, eventA)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 1 || res.Results[0].Status != StatusSkippedDuplicate {
			t.Fatalf("expected StatusSkippedDuplicate, got %+v", res.Results)
		}
		// Notification count must remain 1
		if len(directRepo.created) != 1 {
			t.Fatalf("expected still 1 notification, got %d", len(directRepo.created))
		}
	})

	// -------------------------------------------------------------
	// TEST C: Cooldown (Send different event ID during active cooldown)
	// -------------------------------------------------------------
	t.Run("TestC_ActionCooldown", func(t *testing.T) {
		eventC := Event{
			ID:         "evt-002",
			Type:       EventUserRegistered,
			UserID:     "user-1",
			User:       &userclient.User{ID: "user-1", Email: "alex@example.com", Username: "Alex", IsActive: true},
			OccurredAt: time.Now().UTC().Add(24 * time.Hour), // 1 day later, cooldown is 7 days
		}

		res, err := engine.HandleEvent(ctx, eventC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 1 || res.Results[0].Status != StatusSkippedCooldown {
			t.Fatalf("expected StatusSkippedCooldown, got %+v", res.Results)
		}
		// Notification count must remain 1
		if len(directRepo.created) != 1 {
			t.Fatalf("expected still 1 notification, got %d", len(directRepo.created))
		}
	})

	// -------------------------------------------------------------
	// TEST D: Disabled rule
	// -------------------------------------------------------------
	t.Run("TestD_DisabledRule", func(t *testing.T) {
		rule.Enabled = false

		eventD := Event{
			ID:         "evt-003",
			Type:       EventUserRegistered,
			UserID:     "user-2",
			User:       &userclient.User{ID: "user-2", Email: "bob@example.com", Username: "Bob", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}

		res, err := engine.HandleEvent(ctx, eventD)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 0 {
			t.Fatalf("expected 0 results for disabled rule, got %d", len(res.Results))
		}
		if len(directRepo.created) != 1 {
			t.Fatalf("expected still 1 notification, got %d", len(directRepo.created))
		}
		rule.Enabled = true
	})

	// -------------------------------------------------------------
	// TEST E: Failure Isolation (Rule A fails, Rule B executes)
	// -------------------------------------------------------------
	t.Run("TestE_FailureIsolation", func(t *testing.T) {
		// Rule A targets non-existent template
		ruleA := createTestRule("rule-fail", "Failing Rule", "tpl-nonexistent", true, domain.ConditionGroup{
			Operator: domain.GroupOperatorAll,
			Conditions: []domain.ConditionNode{
				{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
			},
		}, nil)

		// Rule B targets valid template
		ruleB := createTestRule("rule-ok", "Working Rule", "tpl-e2e-1", true, domain.ConditionGroup{
			Operator: domain.GroupOperatorAll,
			Conditions: []domain.ConditionNode{
				{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
			},
		}, nil)

		ruleRepo.rules = []*domain.AutomationRule{ruleA, ruleB}

		eventE := Event{
			ID:         "evt-004",
			Type:       EventUserRegistered,
			UserID:     "user-3",
			User:       &userclient.User{ID: "user-3", Email: "carol@example.com", Username: "Carol", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}

		res, err := engine.HandleEvent(ctx, eventE)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 2 {
			t.Fatalf("expected 2 rule results, got %d", len(res.Results))
		}

		var failedStatus, executedStatus bool
		for _, r := range res.Results {
			if r.RuleID == "rule-fail" && r.Status == StatusFailed {
				failedStatus = true
			}
			if r.RuleID == "rule-ok" && r.Status == StatusExecuted {
				executedStatus = true
			}
		}

		if !failedStatus || !executedStatus {
			t.Fatalf("expected one failed and one executed result, got %+v", res.Results)
		}
		// 1 original notification from Test A + 1 new from Rule B = 2 total
		if len(directRepo.created) != 2 {
			t.Errorf("expected 2 total notifications, got %d", len(directRepo.created))
		}
	})
}

func TestEngine_E2E_InactivityFlow(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	auditRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(auditRepo)
	log := logger.NewNop()

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditSvc, log)

	// 1. Template
	tpl := &domain.EmailTemplate{
		ID:            "tpl-inactivity-e2e",
		TemplateKey:   "tpl_key_inactivity",
		Name:          "Inactivity Template",
		TemplateType:  domain.TemplateTypeAutomation,
		Status:        domain.TemplateStatusActive,
		Subject:       "We miss you {{username}}",
		HTMLBody:      "<p>Come back {{username}}!</p>",
		PlainTextBody: "Come back {{username}}!",
	}
	templateRepo.templates["tpl-inactivity-e2e"] = tpl

	// 2. Inactivity Rule (older_than 30 days, cooldown 7 days)
	cooldownDays := 7
	rule := createTestRule("rule-inactivity-e2e", "Inactive Users Notice", "tpl-inactivity-e2e", true, domain.ConditionGroup{
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
	}, &cooldownDays)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.AddDate(0, 0, -45) // 45 days ago (> 30d)
	recentTime := refTime.AddDate(0, 0, -5)    // 5 days ago (< 30d)

	userLister := &mockUserLister{
		pages: [][]*userclient.User{
			{
				{ID: "user-inactive", Email: "inactive@example.com", Username: "InactiveUser", LastActivityAt: &inactiveTime, IsActive: true},
				{ID: "user-active", Email: "active@example.com", Username: "ActiveUser", LastActivityAt: &recentTime, IsActive: true},
				{ID: "user-null-act", Email: "nullact@example.com", Username: "NullActUser", LastActivityAt: nil, IsActive: true},
			},
		},
	}

	ctx := context.Background()

	// -------------------------------------------------------------
	// STEP 1: First Evaluation Pass
	// -------------------------------------------------------------
	summary1, err := engine.EvaluateInactivity(ctx, userLister, refTime, 50)
	if err != nil {
		t.Fatalf("unexpected error on first pass: %v", err)
	}

	if summary1.TotalUsersEvaluated != 3 {
		t.Errorf("expected 3 users evaluated, got %d", summary1.TotalUsersEvaluated)
	}
	if summary1.ExecutedCount != 1 {
		t.Errorf("expected 1 executed, got %d", summary1.ExecutedCount)
	}
	if summary1.SkippedNotMatchedCount != 2 {
		t.Errorf("expected 2 skipped not matched, got %d", summary1.SkippedNotMatchedCount)
	}
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 direct notification created, got %d", len(directRepo.created))
	}
	if directRepo.created[0].RecipientEmail != "inactive@example.com" {
		t.Errorf("expected notification recipient inactive@example.com, got %s", directRepo.created[0].RecipientEmail)
	}
	if directRepo.created[0].RecipientName != "InactiveUser" {
		t.Errorf("expected recipient name InactiveUser, got %s", directRepo.created[0].RecipientName)
	}

	// -------------------------------------------------------------
	// STEP 2: Second Evaluation Pass (Immediate / Inside 7-day cooldown)
	// -------------------------------------------------------------
	summary2, err := engine.EvaluateInactivity(ctx, userLister, refTime.Add(time.Hour), 50)
	if err != nil {
		t.Fatalf("unexpected error on second pass: %v", err)
	}
	if summary2.ExecutedCount != 0 {
		t.Errorf("expected 0 executed on second pass inside cooldown, got %d", summary2.ExecutedCount)
	}
	if summary2.SkippedCooldownCount != 1 {
		t.Errorf("expected 1 skipped cooldown, got %d", summary2.SkippedCooldownCount)
	}
	if summary2.SkippedNotMatchedCount != 2 {
		t.Errorf("expected 2 skipped not matched, got %d", summary2.SkippedNotMatchedCount)
	}
	// Notification count remains exactly 1
	if len(directRepo.created) != 1 {
		t.Errorf("expected still 1 notification, got %d", len(directRepo.created))
	}

	// -------------------------------------------------------------
	// STEP 3: Third Evaluation Pass (After 8 days - cooldown elapsed)
	// -------------------------------------------------------------
	summary3, err := engine.EvaluateInactivity(ctx, userLister, refTime.AddDate(0, 0, 8), 50)
	if err != nil {
		t.Fatalf("unexpected error on third pass after cooldown: %v", err)
	}
	if summary3.ExecutedCount != 1 {
		t.Errorf("expected 1 executed on third pass after cooldown expired, got %d", summary3.ExecutedCount)
	}
	if len(directRepo.created) != 2 {
		t.Errorf("expected 2 notifications after cooldown elapsed, got %d", len(directRepo.created))
	}
}
