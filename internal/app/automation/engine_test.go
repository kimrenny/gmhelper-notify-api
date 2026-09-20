package automation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

// Mock Rule Repo
type mockRuleRepo struct {
	rules []*domain.AutomationRule
	err   error
}

func (m *mockRuleRepo) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockRuleRepo) Create(ctx context.Context, rule *domain.AutomationRule) error { return nil }
func (m *mockRuleRepo) Update(ctx context.Context, rule *domain.AutomationRule) error { return nil }
func (m *mockRuleRepo) Delete(ctx context.Context, id string) error                   { return nil }
func (m *mockRuleRepo) List(ctx context.Context) ([]*domain.AutomationRule, error) {
	return m.rules, m.err
}
func (m *mockRuleRepo) ListEnabled(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	enabled := []*domain.AutomationRule{}
	for _, r := range m.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled, nil
}

// Mock Template Repo
type engineMockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *engineMockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if tpl, ok := m.templates[id]; ok {
		return tpl, nil
	}
	return nil, domain.ErrNotFound
}
func (m *engineMockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}
func (m *engineMockTemplateRepo) Create(ctx context.Context, tpl *domain.EmailTemplate) error {
	return nil
}
func (m *engineMockTemplateRepo) Update(ctx context.Context, tpl *domain.EmailTemplate) error {
	return nil
}
func (m *engineMockTemplateRepo) Delete(ctx context.Context, id string) error { return nil }
func (m *engineMockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

// Mock Direct Notification Repo
type mockDirectRepo struct {
	created []*domain.DirectNotification
}

func (m *mockDirectRepo) Create(ctx context.Context, notification *domain.DirectNotification) error {
	m.created = append(m.created, notification)
	return nil
}
func (m *mockDirectRepo) CreateWithInitialAttempt(ctx context.Context, notification *domain.DirectNotification, attempt *domain.DeliveryAttempt) error {
	m.created = append(m.created, notification)
	return nil
}
func (m *mockDirectRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	return nil, domain.ErrNotFound
}
func (m *mockDirectRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttempt, sentAt *time.Time, errMsg string) error {
	return nil
}
func (m *mockDirectRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	return nil, nil
}
func (m *mockDirectRepo) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	return nil, nil
}
func (m *mockDirectRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	return 0, nil
}

// Mock Execution Repo
type mockExecRepo struct {
	mu         sync.Mutex
	executions []*domain.AutomationExecution
	directRepo *mockDirectRepo
}

func (m *mockExecRepo) RecordExecution(ctx context.Context, exec *domain.AutomationExecution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions = append(m.executions, exec)
	return nil
}

func (m *mockExecRepo) HasExecution(ctx context.Context, ruleID, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.executions {
		if e.RuleID == ruleID && e.EventID == eventID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockExecRepo) GetLastSuccessfulExecution(ctx context.Context, ruleID, recipientEmail string, externalUserID *string) (*domain.AutomationExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.executions) - 1; i >= 0; i-- {
		e := m.executions[i]
		if e.RuleID == ruleID && e.Status == "success" {
			if e.RecipientEmail == recipientEmail || (externalUserID != nil && e.ExternalUserID != nil && *e.ExternalUserID == *externalUserID) {
				return e, nil
			}
		}
	}
	return nil, nil
}

func (m *mockExecRepo) ExecuteRuleAtomic(
	ctx context.Context,
	ruleID, eventID, recipientEmail string,
	externalUserID *string,
	cooldownDays *int,
	eventTime time.Time,
	notif *domain.DirectNotification,
	exec *domain.AutomationExecution,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.executions {
		if e.RuleID == ruleID && e.EventID == eventID {
			return "skipped_duplicate", nil
		}
	}

	if cooldownDays != nil && *cooldownDays > 0 {
		cooldownDuration := time.Duration(*cooldownDays) * 24 * time.Hour
		var lastExec *domain.AutomationExecution
		for i := len(m.executions) - 1; i >= 0; i-- {
			e := m.executions[i]
			if e.RuleID == ruleID && e.Status == "success" {
				if e.RecipientEmail == recipientEmail || (externalUserID != nil && e.ExternalUserID != nil && *e.ExternalUserID == *externalUserID) {
					lastExec = e
					break
				}
			}
		}
		if lastExec != nil {
			elapsed := eventTime.Sub(lastExec.ExecutedAt)
			if elapsed >= 0 && elapsed < cooldownDuration {
				skipped := &domain.AutomationExecution{
					ID:             exec.ID,
					RuleID:         ruleID,
					EventID:        eventID,
					RecipientEmail: recipientEmail,
					ExternalUserID: externalUserID,
					Status:         "skipped_cooldown",
					ExecutedAt:     eventTime,
					CreatedAt:      time.Now().UTC(),
				}
				m.executions = append(m.executions, skipped)
				return "skipped_cooldown", nil
			}
		}
	}

	if notif != nil && m.directRepo != nil {
		_ = m.directRepo.Create(ctx, notif)
	}
	if exec != nil {
		m.executions = append(m.executions, exec)
	}
	return "success", nil
}

// Mock User Resolver
type mockUserResolver struct {
	users map[string]*userclient.User
}

func (m *mockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, userclient.ErrNotFound
}

// Mock Activity Repo
type mockActivityRepo struct {
	records []*domain.ActivityLog
}

func (m *mockActivityRepo) Create(ctx context.Context, log *domain.ActivityLog) error {
	m.records = append(m.records, log)
	return nil
}
func (m *mockActivityRepo) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}
func (m *mockActivityRepo) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return m.records, len(m.records), nil
}

func createTestRule(id, name, templateID string, enabled bool, conditions domain.ConditionGroup, cooldownDays *int) *domain.AutomationRule {
	return &domain.AutomationRule{
		ID:         id,
		Name:       name,
		TemplateID: templateID,
		Enabled:    enabled,
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
			Conditions: conditions,
			Action: domain.ActionConfig{
				Type:         domain.ActionTypeSendEmail,
				CooldownDays: cooldownDays,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestEngine_SingleMatchingRule(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-welcome": {
				ID:            "tpl-welcome",
				TemplateKey:   "welcome_email",
				Name:          "Welcome Email",
				Subject:       "Welcome {{username}}!",
				HTMLBody:      "<p>Welcome to GMHelper, {{username}}!</p>",
				PlainTextBody: "Welcome to GMHelper, {{username}}!",
				Locale:        "en",
				Status:        domain.TemplateStatusActive,
				TemplateType:  domain.TemplateTypeAutomation,
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	userResolver := &mockUserResolver{}
	activityRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(activityRepo)

	rule := createTestRule("rule-1", "Welcome Active Users", "tpl-welcome", true, domain.ConditionGroup{
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
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, userResolver, auditSvc, logger.NewNop())

	event := Event{
		ID:     "evt-100",
		Type:   EventUserRegistered,
		UserID: "u-1",
		User: &userclient.User{
			ID:       "u-1",
			Username: "alice",
			Email:    "alice@example.com",
			IsActive: true,
		},
		OccurredAt: time.Now().UTC(),
	}

	result, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Status != StatusExecuted {
		t.Errorf("expected StatusExecuted, got %s (err: %s)", result.Results[0].Status, result.Results[0].Error)
	}

	// Verify direct notification created
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 created notification, got %d", len(directRepo.created))
	}
	notif := directRepo.created[0]
	if notif.RecipientEmail != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", notif.RecipientEmail)
	}
	if notif.NotificationType != domain.NotificationTypeAutomation {
		t.Errorf("expected automation notification type, got %s", notif.NotificationType)
	}
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected pending delivery status, got %s", notif.DeliveryStatus)
	}

	// Verify Activity History system audit recorded
	if len(activityRepo.records) != 1 {
		t.Fatalf("expected 1 activity audit log, got %d", len(activityRepo.records))
	}
	act := activityRepo.records[0]
	if act.EventType != domain.EventAutomationExecuted {
		t.Errorf("expected %s, got %s", domain.EventAutomationExecuted, act.EventType)
	}
	if act.ActorType != domain.ActorTypeSystem {
		t.Errorf("expected system actor, got %s", act.ActorType)
	}
}

func TestEngine_NonMatchingAndDisabledRules(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Hi",
				HTMLBody:     "<p>Hi</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	auditRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(auditRepo)

	// Rule 1: Non-matching (requires role "Admin", user has "user")
	rule1 := createTestRule("rule-1", "Admin Rule", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldRole,
					Operator: domain.OperatorEquals,
					Value:    "admin",
				},
			},
		},
	}, nil)

	// Rule 2: Disabled rule
	rule2 := createTestRule("rule-2", "Disabled Rule", "tpl-1", false, domain.ConditionGroup{
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
	}, nil)

	ruleRepo.rules = []*domain.AutomationRule{rule1, rule2}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditSvc, logger.NewNop())

	event := Event{
		ID:     "evt-200",
		Type:   EventUserRegistered,
		UserID: "u-2",
		User: &userclient.User{
			ID:       "u-2",
			Email:    "bob@example.com",
			Role:     "user",
			IsActive: true,
		},
	}

	result, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Disabled rule is not loaded by ListEnabled; non-matching rule is skipped
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result from enabled rule, got %d", len(result.Results))
	}
	if result.Results[0].Status != StatusSkippedNotMatched {
		t.Errorf("expected StatusSkippedNotMatched, got %s", result.Results[0].Status)
	}
	if len(directRepo.created) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(directRepo.created))
	}
}

func TestEngine_FailureIsolation_MultipleRules(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-broken": {
				ID:            "tpl-broken",
				Status:        domain.TemplateStatusActive,
				TemplateType:  domain.TemplateTypeAutomation,
				Subject:       "Missing {{required_var}}",
				HTMLBody:      "<p>Test</p>",
				PlainTextBody: "Test",
			},
			"tpl-valid": {
				ID:            "tpl-valid",
				Status:        domain.TemplateStatusActive,
				TemplateType:  domain.TemplateTypeAutomation,
				Subject:       "Hello {{username}}",
				HTMLBody:      "<p>Hello {{username}}</p>",
				PlainTextBody: "Hello {{username}}",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	auditRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(auditRepo)

	// Rule A: Broken template (missing required_var in context)
	ruleA := createTestRule("rule-a", "Broken Rule", "tpl-broken", true, domain.ConditionGroup{
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
	}, nil)

	// Rule B: Valid rule
	ruleB := createTestRule("rule-b", "Working Rule", "tpl-valid", true, domain.ConditionGroup{
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
	}, nil)

	ruleRepo.rules = []*domain.AutomationRule{ruleA, ruleB}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditSvc, logger.NewNop())

	event := Event{
		ID:     "evt-300",
		Type:   EventUserRegistered,
		UserID: "u-3",
		User: &userclient.User{
			ID:       "u-3",
			Username: "charlie",
			Email:    "charlie@example.com",
			IsActive: true,
		},
	}

	result, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	// Rule A fails due to strict missing variable check
	if result.Results[0].Status != StatusFailed {
		t.Errorf("expected Rule A to fail, got %s", result.Results[0].Status)
	}

	// Rule B succeeds despite Rule A failure (Failure Isolation)
	if result.Results[1].Status != StatusExecuted {
		t.Errorf("expected Rule B to execute, got %s (err: %s)", result.Results[1].Status, result.Results[1].Error)
	}

	// Exactly 1 notification created for Rule B
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 notification created, got %d", len(directRepo.created))
	}

	// 2 audit events: 1 failure for Rule A, 1 success for Rule B
	if len(auditRepo.records) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(auditRepo.records))
	}
	if auditRepo.records[0].EventType != domain.EventAutomationFailed {
		t.Errorf("expected failure audit for rule A, got %s", auditRepo.records[0].EventType)
	}
	if auditRepo.records[1].EventType != domain.EventAutomationExecuted {
		t.Errorf("expected success audit for rule B, got %s", auditRepo.records[1].EventType)
	}
}

func TestEngine_IdempotencyAndCooldown(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Notice",
				HTMLBody:     "<p>Notice</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	auditRepo := &mockActivityRepo{}
	auditSvc := audit.NewService(auditRepo)

	cooldownDays := 7
	rule := createTestRule("rule-1", "Weekly Digest", "tpl-1", true, domain.ConditionGroup{
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
	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditSvc, logger.NewNop())

	now := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	event1 := Event{
		ID:     "evt-same-id",
		Type:   EventUserRegistered,
		UserID: "u-10",
		User: &userclient.User{
			ID:       "u-10",
			Email:    "dave@example.com",
			IsActive: true,
		},
		OccurredAt: now,
	}

	// 1. First execution -> succeeds
	res1, err := engine.HandleEvent(context.Background(), event1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.Results[0].Status != StatusExecuted {
		t.Fatalf("expected StatusExecuted, got %s", res1.Results[0].Status)
	}
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(directRepo.created))
	}

	// 2. Exact same event re-delivered -> Skipped as duplicate (Idempotency)
	res2, err := engine.HandleEvent(context.Background(), event1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Results[0].Status != StatusSkippedDuplicate {
		t.Fatalf("expected StatusSkippedDuplicate, got %s", res2.Results[0].Status)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected still 1 notification after duplicate, got %d", len(directRepo.created))
	}

	// 3. Different event 3 days later (< 7 day cooldown) -> Skipped due to Cooldown
	event2 := Event{
		ID:     "evt-diff-id-1",
		Type:   EventUserRegistered,
		UserID: "u-10",
		User: &userclient.User{
			ID:       "u-10",
			Email:    "dave@example.com",
			IsActive: true,
		},
		OccurredAt: now.Add(3 * 24 * time.Hour),
	}
	res3, err := engine.HandleEvent(context.Background(), event2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res3.Results[0].Status != StatusSkippedCooldown {
		t.Fatalf("expected StatusSkippedCooldown, got %s", res3.Results[0].Status)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected still 1 notification during cooldown, got %d", len(directRepo.created))
	}

	// 4. Different event 8 days later (> 7 day cooldown) -> Executes successfully
	event3 := Event{
		ID:     "evt-diff-id-2",
		Type:   EventUserRegistered,
		UserID: "u-10",
		User: &userclient.User{
			ID:       "u-10",
			Email:    "dave@example.com",
			IsActive: true,
		},
		OccurredAt: now.Add(8 * 24 * time.Hour),
	}
	res4, err := engine.HandleEvent(context.Background(), event3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.Results[0].Status != StatusExecuted {
		t.Fatalf("expected StatusExecuted after cooldown, got %s (err: %s)", res4.Results[0].Status, res4.Results[0].Error)
	}
	if len(directRepo.created) != 2 {
		t.Errorf("expected 2 total notifications after cooldown expired, got %d", len(directRepo.created))
	}
}

func TestEngine_UserResolution(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}",
				HTMLBody:     "<p>Welcome</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	userResolver := &mockUserResolver{
		users: map[string]*userclient.User{
			"ext-user-99": {
				ID:       "ext-user-99",
				Username: "eva",
				Email:    "eva@example.com",
				IsActive: true,
				Role:     "user",
			},
		},
	}

	rule := createTestRule("rule-1", "User Rule", "tpl-1", true, domain.ConditionGroup{
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
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, userResolver, nil, logger.NewNop())

	// Event provides UserID only, User object is nil
	event := Event{
		ID:     "evt-resolve",
		Type:   EventUserRegistered,
		UserID: "ext-user-99",
	}

	result, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Status != StatusExecuted {
		t.Fatalf("expected StatusExecuted after user resolution, got %v", result)
	}

	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 notification created, got %d", len(directRepo.created))
	}
	if directRepo.created[0].RecipientEmail != "eva@example.com" {
		t.Errorf("expected eva@example.com, got %s", directRepo.created[0].RecipientEmail)
	}
}

func TestEngine_ConcurrentDuplicateEvents(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}",
				HTMLBody:     "<p>Welcome</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	rule := createTestRule("rule-1", "Concurrent Rule", "tpl-1", true, domain.ConditionGroup{
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
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	event := Event{
		ID:         "evt-concurrent-duplicate",
		Type:       EventUserRegistered,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "conc@example.com", Username: "Conc", IsActive: true},
		OccurredAt: time.Now().UTC(),
	}

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]*EventExecutionResult, goroutines)
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, err := engine.HandleEvent(context.Background(), event)
			results[idx] = res
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	executedCount := 0
	duplicateCount := 0

	for i := 0; i < goroutines; i++ {
		if errors[i] != nil {
			t.Fatalf("goroutine %d failed with unexpected error: %v", i, errors[i])
		}
		if len(results[i].Results) != 1 {
			t.Fatalf("goroutine %d returned %d results", i, len(results[i].Results))
		}
		switch results[i].Results[0].Status {
		case StatusExecuted:
			executedCount++
		case StatusSkippedDuplicate:
			duplicateCount++
		default:
			t.Fatalf("goroutine %d returned unexpected status: %s", i, results[i].Results[0].Status)
		}
	}

	if executedCount != 1 {
		t.Errorf("expected exactly 1 execution to succeed, got %d", executedCount)
	}
	if duplicateCount != goroutines-1 {
		t.Errorf("expected %d duplicate skips, got %d", goroutines-1, duplicateCount)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected exactly 1 direct notification created in DB, got %d", len(directRepo.created))
	}
}

func TestEngine_ConcurrentCooldownEvents(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}",
				HTMLBody:     "<p>Welcome</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	cooldownDays := 7
	rule := createTestRule("rule-1", "Cooldown Rule", "tpl-1", true, domain.ConditionGroup{
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

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]*EventExecutionResult, goroutines)
	errors := make([]error, goroutines)
	now := time.Now().UTC()

	// Each goroutine receives a unique event ID for the same user
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			event := Event{
				ID:         time.Now().Format("150405.000000000") + string(rune('A'+idx)),
				Type:       EventUserRegistered,
				UserID:     "user-cd-1",
				User:       &userclient.User{ID: "user-cd-1", Email: "cd@example.com", Username: "CD", IsActive: true},
				OccurredAt: now,
			}
			res, err := engine.HandleEvent(context.Background(), event)
			results[idx] = res
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	executedCount := 0
	cooldownCount := 0

	for i := 0; i < goroutines; i++ {
		if errors[i] != nil {
			t.Fatalf("goroutine %d failed with error: %v", i, errors[i])
		}
		if len(results[i].Results) != 1 {
			t.Fatalf("goroutine %d returned %d results", i, len(results[i].Results))
		}
		switch results[i].Results[0].Status {
		case StatusExecuted:
			executedCount++
		case StatusSkippedCooldown:
			cooldownCount++
		default:
			t.Fatalf("goroutine %d returned unexpected status: %s", i, results[i].Results[0].Status)
		}
	}

	if executedCount != 1 {
		t.Errorf("expected exactly 1 execution before cooldown, got %d", executedCount)
	}
	if cooldownCount != goroutines-1 {
		t.Errorf("expected %d cooldown skips, got %d", goroutines-1, cooldownCount)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected exactly 1 direct notification created, got %d", len(directRepo.created))
	}
}

func TestEngine_TemplateFailure_AllowsSubsequentRetry(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-broken": {
				ID:           "tpl-broken",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{missing_variable}}",
				HTMLBody:     "<p>Welcome {{missing_variable}}</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	rule := createTestRule("rule-1", "Broken Rule", "tpl-broken", true, domain.ConditionGroup{
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
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	event := Event{
		ID:         "evt-retry-1",
		Type:       EventUserRegistered,
		UserID:     "user-retry",
		User:       &userclient.User{ID: "user-retry", Email: "retry@example.com", Username: "Retry", IsActive: true},
		OccurredAt: time.Now().UTC(),
	}

	// 1. Initial attempt fails due to missing template variable
	res1, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.Results[0].Status != StatusFailed {
		t.Fatalf("expected StatusFailed on missing variable, got %s", res1.Results[0].Status)
	}
	if len(directRepo.created) != 0 {
		t.Fatalf("expected 0 notifications after failure, got %d", len(directRepo.created))
	}

	// 2. Fix the template
	templateRepo.templates["tpl-broken"] = &domain.EmailTemplate{
		ID:           "tpl-broken",
		Status:       domain.TemplateStatusActive,
		TemplateType: domain.TemplateTypeAutomation,
		Subject:      "Welcome {{username}}",
		HTMLBody:     "<p>Welcome {{username}}</p>",
	}

	// 3. Retry the exact same event ID -> should succeed and not be blocked as duplicate
	res2, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if res2.Results[0].Status != StatusExecuted {
		t.Fatalf("expected StatusExecuted on retry, got %s (err: %s)", res2.Results[0].Status, res2.Results[0].Error)
	}
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 notification created after successful retry, got %d", len(directRepo.created))
	}
}

func TestEngine_IncompatibleTemplateType(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-campaign": {
				ID:           "tpl-campaign",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeCampaign, // Incompatible with automation
				Subject:      "Campaign subject",
				HTMLBody:     "<p>Body</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	rule := createTestRule("rule-1", "Campaign Rule", "tpl-campaign", true, domain.ConditionGroup{
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
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	event := Event{
		ID:         "evt-incompatible-tpl",
		Type:       EventUserRegistered,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "incompat@example.com", Username: "Incompat", IsActive: true},
		OccurredAt: time.Now().UTC(),
	}

	res, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Results[0].Status != StatusFailed {
		t.Fatalf("expected StatusFailed for incompatible template type, got %s", res.Results[0].Status)
	}
	if len(directRepo.created) != 0 {
		t.Fatalf("expected 0 notifications, got %d", len(directRepo.created))
	}
}

type mockUserLister struct {
	pages [][]*userclient.User
	err   error
}

func (m *mockUserLister) GetUsers(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
	if m.err != nil {
		return nil, m.err
	}
	if page <= 0 || page > len(m.pages) {
		return &userclient.PagedUsers{
			Items:       []userclient.User{},
			Page:        page,
			PageSize:    pageSize,
			TotalCount:  0,
			HasNextPage: false,
		}, nil
	}
	items := make([]userclient.User, len(m.pages[page-1]))
	for i, u := range m.pages[page-1] {
		if u != nil {
			items[i] = *u
		}
	}
	total := 0
	for _, p := range m.pages {
		total += len(p)
	}
	return &userclient.PagedUsers{
		Items:       items,
		Page:        page,
		PageSize:    pageSize,
		TotalCount:  total,
		HasNextPage: page < len(m.pages),
	}, nil
}

func TestEngine_EvaluateInactivity_SuccessAndCooldown(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cooldown := 7
	inactiveTime := refTime.AddDate(0, 0, -45)
	activeTime := refTime.AddDate(0, 0, -5)

	rule := createTestRule("rule-inactivity", "Inactive User Rule", "tpl-1", true, domain.ConditionGroup{
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
	}, &cooldown)

	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{rule}}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "We miss you {{username}}",
				HTMLBody:     "<p>Hello {{username}}, please come back!</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	userLister := &mockUserLister{
		pages: [][]*userclient.User{
			{
				{ID: "user-1", Email: "inactive@example.com", Username: "InactiveUser", LastActivityAt: &inactiveTime, IsActive: true},
				{ID: "user-2", Email: "active@example.com", Username: "ActiveUser", LastActivityAt: &activeTime, IsActive: true},
				{ID: "user-3", Email: "noactivity@example.com", Username: "NoActivityUser", LastActivityAt: nil, IsActive: true},
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	// First evaluation pass
	summary, err := engine.EvaluateInactivity(context.Background(), userLister, refTime, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalUsersEvaluated != 3 {
		t.Errorf("expected 3 users evaluated, got %d", summary.TotalUsersEvaluated)
	}
	if summary.TotalRulesEvaluated != 1 {
		t.Errorf("expected 1 rule evaluated, got %d", summary.TotalRulesEvaluated)
	}
	if summary.ExecutedCount != 1 {
		t.Errorf("expected 1 executed, got %d", summary.ExecutedCount)
	}
	if summary.SkippedNotMatchedCount != 2 {
		t.Errorf("expected 2 skipped not matched, got %d", summary.SkippedNotMatchedCount)
	}
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 direct notification created, got %d", len(directRepo.created))
	}
	if directRepo.created[0].RecipientEmail != "inactive@example.com" {
		t.Errorf("expected notification for inactive@example.com, got %s", directRepo.created[0].RecipientEmail)
	}

	// Second evaluation pass (within cooldown)
	summary2, err := engine.EvaluateInactivity(context.Background(), userLister, refTime.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("unexpected error on second pass: %v", err)
	}

	if summary2.ExecutedCount != 0 {
		t.Errorf("expected 0 executed on second pass inside cooldown, got %d", summary2.ExecutedCount)
	}
	if summary2.SkippedCooldownCount != 1 {
		t.Errorf("expected 1 skipped cooldown on second pass, got %d", summary2.SkippedCooldownCount)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected still 1 notification created, got %d", len(directRepo.created))
	}
}

func TestEngine_EvaluateInactivity_DuplicateWithoutCooldown(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.AddDate(0, 0, -45)

	// Cooldown is nil / 0
	rule := createTestRule("rule-inactivity-no-cd", "Inactive User Rule", "tpl-1", true, domain.ConditionGroup{
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
				{ID: "user-1", Email: "inactive@example.com", Username: "InactiveUser", LastActivityAt: &inactiveTime, IsActive: true},
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	// Pass 1
	summary1, err := engine.EvaluateInactivity(context.Background(), userLister, refTime, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary1.ExecutedCount != 1 {
		t.Fatalf("expected 1 executed, got %d", summary1.ExecutedCount)
	}

	// Pass 2 (immediate or next tick) -> deterministic event ID deduplication skips it
	summary2, err := engine.EvaluateInactivity(context.Background(), userLister, refTime.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary2.ExecutedCount != 0 {
		t.Errorf("expected 0 executed on second pass, got %d", summary2.ExecutedCount)
	}
	if summary2.SkippedDuplicateCount != 1 {
		t.Errorf("expected 1 skipped (duplicate), got %d", summary2.SkippedDuplicateCount)
	}
	if len(directRepo.created) != 1 {
		t.Errorf("expected 1 direct notification, got %d", len(directRepo.created))
	}
}

func TestEngine_EvaluateInactivity_ErrorDoesNotStopBatch(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.AddDate(0, 0, -45)

	// Rule references non-existent template
	rule := createTestRule("rule-inactivity-bad-tpl", "Inactive Rule", "tpl-missing", true, domain.ConditionGroup{
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
	templateRepo := &engineMockTemplateRepo{templates: map[string]*domain.EmailTemplate{}}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	userLister := &mockUserLister{
		pages: [][]*userclient.User{
			{
				{ID: "user-1", Email: "inactive1@example.com", Username: "User1", LastActivityAt: &inactiveTime, IsActive: true},
				{ID: "user-2", Email: "inactive2@example.com", Username: "User2", LastActivityAt: &inactiveTime, IsActive: true},
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	summary, err := engine.EvaluateInactivity(context.Background(), userLister, refTime, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalUsersEvaluated != 2 {
		t.Errorf("expected 2 users evaluated, got %d", summary.TotalUsersEvaluated)
	}
	if summary.FailedCount != 2 {
		t.Errorf("expected 2 failed due to missing template, got %d", summary.FailedCount)
	}
	if summary.ExecutedCount != 0 {
		t.Errorf("expected 0 executed, got %d", summary.ExecutedCount)
	}
}
