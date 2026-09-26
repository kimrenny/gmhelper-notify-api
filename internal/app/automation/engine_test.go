package automation

import (
	"context"
	"encoding/json"
	"strings"
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
func (m *mockRuleRepo) Create(ctx context.Context, rule *domain.AutomationRule) error {
	cp := *rule
	m.rules = append(m.rules, &cp)
	return nil
}
func (m *mockRuleRepo) Update(ctx context.Context, rule *domain.AutomationRule) error {
	for i, r := range m.rules {
		if r.ID == rule.ID {
			cp := *rule
			m.rules[i] = &cp
			return nil
		}
	}
	return domain.ErrNotFound
}
func (m *mockRuleRepo) UpdateEvaluationTimes(ctx context.Context, id string, lastEvaluatedAt, nextEvaluationAt *time.Time) error {
	for _, r := range m.rules {
		if r.ID == id {
			r.LastEvaluatedAt = lastEvaluatedAt
			r.NextEvaluationAt = nextEvaluationAt
			return nil
		}
	}
	return nil
}
func (m *mockRuleRepo) Delete(ctx context.Context, id string) error { return nil }
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
	for _, tpl := range m.templates {
		if tpl.TemplateKey == templateKey {
			return tpl, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *engineMockTemplateRepo) GetByKeyAndLocale(ctx context.Context, templateKey, locale string) (*domain.EmailTemplate, error) {
	for _, tpl := range m.templates {
		if tpl.TemplateKey == templateKey && (tpl.Locale == locale || locale == "") {
			return tpl, nil
		}
	}
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

func (m *mockExecRepo) ListByRuleID(ctx context.Context, ruleID string, limit, offset int) ([]*domain.AutomationExecution, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []*domain.AutomationExecution
	for _, e := range m.executions {
		if e.RuleID == ruleID {
			filtered = append(filtered, e)
		}
	}
	total := len(filtered)
	if offset >= total {
		return []*domain.AutomationExecution{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
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
			Trigger: domain.TriggerUserRegistered,
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
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func createTestInactivityRule(id, name, templateID string, enabled bool, conditions domain.ConditionGroup, cooldownDays *int) *domain.AutomationRule {
	r := createTestRule(id, name, templateID, enabled, conditions, cooldownDays)
	r.Config.Trigger = domain.TriggerUserInactive
	return r
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
	userResolver := &mockUserResolver{
		users: map[string]*userclient.User{
			"u-1": {
				ID:       "u-1",
				Username: "alice",
				Email:    "alice@example.com",
				IsActive: true,
			},
		},
	}
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

	rule := createTestInactivityRule("rule-inactivity", "Inactive User Rule", "tpl-1", true, domain.ConditionGroup{
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

	// Same-day evaluation pass (1 hour later) -> Rule is NOT due
	summarySameDay, err := engine.EvaluateInactivity(context.Background(), userLister, refTime.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("unexpected error on same day pass: %v", err)
	}
	if summarySameDay.TotalRulesEvaluated != 0 {
		t.Errorf("expected 0 rules evaluated on same day tick, got %d", summarySameDay.TotalRulesEvaluated)
	}

	// Next day evaluation pass (within 30-day cooldown) -> Rule IS due, user skipped on cooldown
	nextDayTime := refTime.Add(21 * time.Hour).Add(5 * time.Minute) // 2026-09-21 09:05:00 UTC
	summary2, err := engine.EvaluateInactivity(context.Background(), userLister, nextDayTime, 10)
	if err != nil {
		t.Fatalf("unexpected error on second pass: %v", err)
	}
	if summary2.TotalRulesEvaluated != 1 {
		t.Errorf("expected 1 rule evaluated on next day pass, got %d", summary2.TotalRulesEvaluated)
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
	rule := createTestInactivityRule("rule-inactivity-no-cd", "Inactive User Rule", "tpl-1", true, domain.ConditionGroup{
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

	// Pass 2: Next day -> Rule IS due, but deterministic event ID deduplication skips it
	nextDayTime := refTime.Add(21 * time.Hour).Add(5 * time.Minute)
	summary2, err := engine.EvaluateInactivity(context.Background(), userLister, nextDayTime, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary2.TotalRulesEvaluated != 1 {
		t.Errorf("expected 1 rule evaluated next day, got %d", summary2.TotalRulesEvaluated)
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
	rule := createTestInactivityRule("rule-inactivity-bad-tpl", "Inactive Rule", "tpl-missing", true, domain.ConditionGroup{
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

func TestEngine_TriggerMatching_EventTypes(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-reg":      {ID: "tpl-reg", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Reg", HTMLBody: "<p>Reg</p>"},
			"tpl-email":    {ID: "tpl-email", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Email", HTMLBody: "<p>Email</p>"},
			"tpl-pwd":      {ID: "tpl-pwd", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Pwd", HTMLBody: "<p>Pwd</p>"},
			"tpl-blocked":  {ID: "tpl-blocked", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Blocked", HTMLBody: "<p>Blocked</p>"},
			"tpl-unblock":  {ID: "tpl-unblock", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Unblock", HTMLBody: "<p>Unblock</p>"},
			"tpl-lang":     {ID: "tpl-lang", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Lang", HTMLBody: "<p>Lang</p>"},
			"tpl-inactive": {ID: "tpl-inactive", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Inactive", HTMLBody: "<p>Inactive</p>"},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	allTrueConditions := domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
		},
	}

	makeRuleWithTrigger := func(id, trigger, tplID string) *domain.AutomationRule {
		r := createTestRule(id, "Rule "+trigger, tplID, true, allTrueConditions, nil)
		r.Config.Trigger = trigger
		return r
	}

	ruleRepo.rules = []*domain.AutomationRule{
		makeRuleWithTrigger("r-reg", domain.TriggerUserRegistered, "tpl-reg"),
		makeRuleWithTrigger("r-email", domain.TriggerEmailConfirmed, "tpl-email"),
		makeRuleWithTrigger("r-pwd", domain.TriggerPasswordChanged, "tpl-pwd"),
		makeRuleWithTrigger("r-blocked", domain.TriggerUserBlocked, "tpl-blocked"),
		makeRuleWithTrigger("r-unblock", domain.TriggerUserUnblocked, "tpl-unblock"),
		makeRuleWithTrigger("r-lang", domain.TriggerUserLanguageChanged, "tpl-lang"),
		makeRuleWithTrigger("r-inactive", domain.TriggerUserInactive, "tpl-inactive"),
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	testCases := []struct {
		eventType      string
		expectedRuleID string
		expectedTplID  string
	}{
		{domain.TriggerUserRegistered, "r-reg", "tpl-reg"},
		{domain.TriggerEmailConfirmed, "r-email", "tpl-email"},
		{domain.TriggerPasswordChanged, "r-pwd", "tpl-pwd"},
		{domain.TriggerUserBlocked, "r-blocked", "tpl-blocked"},
		{domain.TriggerUserUnblocked, "r-unblock", "tpl-unblock"},
		{domain.TriggerUserLanguageChanged, "r-lang", "tpl-lang"},
	}

	for _, tc := range testCases {
		t.Run("event_"+tc.eventType, func(t *testing.T) {
			directRepo.created = nil
			evt := Event{
				ID:         "evt-" + tc.eventType,
				Type:       tc.eventType,
				UserID:     "user-1",
				User:       &userclient.User{ID: "user-1", Email: "test@example.com", Username: "test", IsActive: true},
				OccurredAt: time.Now().UTC(),
			}

			res, err := engine.HandleEvent(context.Background(), evt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(res.Results) != 1 {
				t.Fatalf("expected exactly 1 evaluated rule result for %s, got %d", tc.eventType, len(res.Results))
			}
			if res.Results[0].RuleID != tc.expectedRuleID {
				t.Errorf("expected rule ID %s, got %s", tc.expectedRuleID, res.Results[0].RuleID)
			}
			if res.Results[0].Status != StatusExecuted {
				t.Errorf("expected StatusExecuted, got %s", res.Results[0].Status)
			}
			if len(directRepo.created) != 1 {
				t.Fatalf("expected 1 notification created, got %d", len(directRepo.created))
			}
			if directRepo.created[0].TemplateID != tc.expectedTplID {
				t.Errorf("expected template %s, got %s", tc.expectedTplID, directRepo.created[0].TemplateID)
			}
		})
	}

	// Test unrelated / unhandled event type
	t.Run("unrelated_event_triggers_nothing", func(t *testing.T) {
		directRepo.created = nil
		evt := Event{
			ID:         "evt-custom",
			Type:       "custom.unknown.event",
			UserID:     "user-1",
			User:       &userclient.User{ID: "user-1", Email: "test@example.com", Username: "test", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}

		res, err := engine.HandleEvent(context.Background(), evt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 0 {
			t.Errorf("expected 0 results for unrelated event, got %d", len(res.Results))
		}
		if len(directRepo.created) != 0 {
			t.Errorf("expected 0 notifications created, got %d", len(directRepo.created))
		}
	})
}

func TestEngine_TriggerMatching_ConditionEvaluation(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-en": {ID: "tpl-en", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "English", HTMLBody: "<p>EN</p>"},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	enCondition := domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldLanguage, Operator: domain.OperatorEquals, Value: "en"}},
		},
	}

	rule := createTestRule("rule-en-reg", "English Reg", "tpl-en", true, enCondition, nil)
	rule.Config.Trigger = domain.TriggerUserRegistered
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	// 1. Matching trigger + Matching conditions -> Executed
	t.Run("matching_trigger_and_matching_conditions", func(t *testing.T) {
		directRepo.created = nil
		evt := Event{
			ID:         "evt-1",
			Type:       domain.TriggerUserRegistered,
			UserID:     "user-en",
			User:       &userclient.User{ID: "user-en", Email: "en@example.com", Username: "en_user", Language: "en", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}
		res, err := engine.HandleEvent(context.Background(), evt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(res.Results))
		}
		if res.Results[0].Status != StatusExecuted {
			t.Errorf("expected StatusExecuted, got %s", res.Results[0].Status)
		}
		if len(directRepo.created) != 1 {
			t.Errorf("expected 1 notification, got %d", len(directRepo.created))
		}
	})

	// 2. Matching trigger + Non-matching conditions -> Skipped
	t.Run("matching_trigger_and_non_matching_conditions", func(t *testing.T) {
		directRepo.created = nil
		evt := Event{
			ID:         "evt-2",
			Type:       domain.TriggerUserRegistered,
			UserID:     "user-pl",
			User:       &userclient.User{ID: "user-pl", Email: "pl@example.com", Username: "pl_user", Language: "pl", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}
		res, err := engine.HandleEvent(context.Background(), evt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(res.Results))
		}
		if res.Results[0].Status != StatusSkippedNotMatched {
			t.Errorf("expected StatusSkippedNotMatched, got %s", res.Results[0].Status)
		}
		if len(directRepo.created) != 0 {
			t.Errorf("expected 0 notifications, got %d", len(directRepo.created))
		}
	})

	// 3. Unrelated trigger (even if condition matches) -> Ignored before evaluation
	t.Run("unrelated_trigger_with_matching_condition_is_ignored", func(t *testing.T) {
		directRepo.created = nil
		evt := Event{
			ID:         "evt-3",
			Type:       domain.TriggerEmailConfirmed,
			UserID:     "user-en",
			User:       &userclient.User{ID: "user-en", Email: "en@example.com", Username: "en_user", Language: "en", IsActive: true},
			OccurredAt: time.Now().UTC(),
		}
		res, err := engine.HandleEvent(context.Background(), evt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Results) != 0 {
			t.Errorf("expected 0 results because trigger does not match, got %d", len(res.Results))
		}
		if len(directRepo.created) != 0 {
			t.Errorf("expected 0 notifications, got %d", len(directRepo.created))
		}
	})
}

func TestEngine_Inactivity_DoesNotEvaluateEventRules(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.AddDate(0, 0, -45)

	eventRule := createTestRule("rule-event", "Event Rule", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
		},
	}, nil)
	eventRule.Config.Trigger = domain.TriggerUserRegistered

	inactivityRule := createTestInactivityRule("rule-inact", "Inactivity Rule", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldLastActivityAt, Operator: domain.OperatorOlderThan, Value: 30, Unit: domain.UnitDays}},
		},
	}, nil)

	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{eventRule, inactivityRule}}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {ID: "tpl-1", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Sub", HTMLBody: "<p>Body</p>"},
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

	// 1. Inactivity evaluation must only evaluate the inactivity rule
	summary, err := engine.EvaluateInactivity(context.Background(), userLister, refTime, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalRulesEvaluated != 1 {
		t.Errorf("expected 1 rule evaluated during inactivity evaluation, got %d", summary.TotalRulesEvaluated)
	}
	if summary.ExecutedCount != 1 {
		t.Errorf("expected 1 executed, got %d", summary.ExecutedCount)
	}

	// 2. HandleEvent must only evaluate the event rule and ignore inactivity rule
	directRepo.created = nil
	evt := Event{
		ID:         "evt-10",
		Type:       domain.TriggerUserRegistered,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "inactive@example.com", Username: "User1", IsActive: true},
		OccurredAt: refTime,
	}
	res, err := engine.HandleEvent(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 result from HandleEvent, got %d", len(res.Results))
	}
	if res.Results[0].RuleID != "rule-event" {
		t.Errorf("expected rule-event to execute, got %s", res.Results[0].RuleID)
	}
}

func TestEngine_MissingAndInvalidTriggersNeverExecute(t *testing.T) {
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	inactiveTime := refTime.Add(-40 * 24 * time.Hour)

	// Rule 1: Missing trigger (empty string)
	missingTriggerRule := createTestRule("rule-missing-trigger", "Missing Trigger", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
		},
	}, nil)
	missingTriggerRule.Config.Trigger = ""

	// Rule 2: Unknown trigger
	unknownTriggerRule := createTestRule("rule-unknown-trigger", "Unknown Trigger", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
		},
	}, nil)
	unknownTriggerRule.Config.Trigger = "unknown.custom_trigger"

	// Rule 3: Legitimate password.changed trigger
	passwordChangedRule := createTestRule("rule-password-changed", "Password Changed", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
		},
	}, nil)
	passwordChangedRule.Config.Trigger = domain.TriggerPasswordChanged

	// Rule 4: Legitimate user.inactive trigger
	inactivityRule := createTestInactivityRule("rule-inactivity", "Inactivity", "tpl-1", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldLastActivityAt, Operator: domain.OperatorOlderThan, Value: 30, Unit: domain.UnitDays}},
		},
	}, nil)

	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{
		missingTriggerRule,
		unknownTriggerRule,
		passwordChangedRule,
		inactivityRule,
	}}

	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {ID: "tpl-1", Status: domain.TemplateStatusActive, TemplateType: domain.TemplateTypeAutomation, Subject: "Sub", HTMLBody: "<p>Body</p>"},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	userLister := &mockUserLister{
		pages: [][]*userclient.User{
			{
				{ID: "user-1", Email: "test@example.com", Username: "User1", LastActivityAt: &inactiveTime, IsActive: true},
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	// 1. Send user.registered event:
	// Missing trigger, unknown trigger, password.changed, and inactivity rules must NOT execute.
	evtReg := Event{
		ID:         "evt-reg-1",
		Type:       domain.TriggerUserRegistered,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "test@example.com", Username: "User1", IsActive: true},
		OccurredAt: refTime,
	}
	resReg, err := engine.HandleEvent(context.Background(), evtReg)
	if err != nil {
		t.Fatalf("unexpected error on HandleEvent user.registered: %v", err)
	}
	if len(resReg.Results) != 0 {
		t.Fatalf("expected 0 executed rules for user.registered event, got %d", len(resReg.Results))
	}

	// 2. Send password.changed event:
	// Only passwordChangedRule should execute. Missing and unknown triggers must NOT execute.
	evtPass := Event{
		ID:         "evt-pass-1",
		Type:       domain.TriggerPasswordChanged,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "test@example.com", Username: "User1", IsActive: true},
		OccurredAt: refTime,
	}
	resPass, err := engine.HandleEvent(context.Background(), evtPass)
	if err != nil {
		t.Fatalf("unexpected error on HandleEvent password.changed: %v", err)
	}
	if len(resPass.Results) != 1 {
		t.Fatalf("expected exactly 1 result for password.changed event, got %d", len(resPass.Results))
	}
	if resPass.Results[0].RuleID != "rule-password-changed" {
		t.Errorf("expected rule-password-changed, got %s", resPass.Results[0].RuleID)
	}

	// 3. EvaluateInactivity:
	// Only inactivityRule should be evaluated and executed. Missing and unknown triggers must NOT be evaluated.
	summary, err := engine.EvaluateInactivity(context.Background(), userLister, refTime, 10)
	if err != nil {
		t.Fatalf("unexpected error on EvaluateInactivity: %v", err)
	}
	if summary.TotalRulesEvaluated != 1 {
		t.Errorf("expected exactly 1 rule evaluated in inactivity scheduler, got %d", summary.TotalRulesEvaluated)
	}
	if summary.ExecutedCount != 1 {
		t.Errorf("expected exactly 1 execution in inactivity scheduler, got %d", summary.ExecutedCount)
	}
}

func TestHandleEvent_SkipsInactiveTriggerRules(t *testing.T) {
	ruleRepo := &mockRuleRepo{
		rules: []*domain.AutomationRule{
			{
				ID:         "rule-inactivity-1",
				Name:       "Inactivity Notification Rule",
				TemplateID: "tpl-1",
				Enabled:    true,
				Config: domain.AutomationRuleConfig{
					Version: 1,
					Trigger: domain.TriggerUserInactive,
					Conditions: domain.ConditionGroup{
						Operator: domain.GroupOperatorAll,
						Conditions: []domain.ConditionNode{
							{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
						},
					},
					Action: domain.ActionConfig{Type: domain.ActionTypeSendEmail},
				},
			},
		},
	}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "We miss you {{username}}",
				HTMLBody:     "<p>Hello {{username}}</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{}
	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	// Even if an external event is sent with Type == "user.inactive", HandleEvent must skip it
	event := Event{
		ID:         "evt-inactive-manual-1",
		Type:       domain.TriggerUserInactive,
		UserID:     "user-1",
		User:       &userclient.User{ID: "user-1", Email: "test@example.com", Username: "Alice", IsActive: true},
		OccurredAt: time.Now().UTC(),
	}

	res, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error on HandleEvent: %v", err)
	}

	if len(res.Results) != 0 {
		t.Fatalf("expected 0 executed rules for scheduled inactivity trigger via HandleEvent, got %d", len(res.Results))
	}
	if len(directRepo.created) != 0 {
		t.Fatalf("expected 0 notifications created, got %d", len(directRepo.created))
	}
}

func TestEngine_AuthoritativeUserResolution_EventExecution(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-auth": {
				ID:           "tpl-auth",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}} - Role: {{role}}",
				HTMLBody:     "<p>Hello {{username}}, your email is {{email}} and role is {{role}}.</p>",
			},
		},
	}
	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	regTime := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	actTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	authoritativeUser := &userclient.User{
		ID:               "usr-authoritative-1",
		Username:         "auth_alice",
		Email:            "authoritative.alice@example.com",
		Role:             "Admin",
		Language:         "EN",
		IsActive:         true,
		IsBlocked:        false,
		RegistrationDate: regTime,
		LastActivityAt:   &actTime,
	}

	userResolver := &mockUserResolver{
		users: map[string]*userclient.User{
			"usr-authoritative-1": authoritativeUser,
		},
	}

	rule := createTestRule("rule-auth-1", "Authoritative Role & Language Rule", "tpl-auth", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{Field: domain.FieldRole, Operator: domain.OperatorEquals, Value: "Admin"}},
			{Item: &domain.ConditionItem{Field: domain.FieldLanguage, Operator: domain.OperatorEquals, Value: "EN"}},
			{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
			{Item: &domain.ConditionItem{Field: domain.FieldIsBlocked, Operator: domain.OperatorEquals, Value: false}},
			{Item: &domain.ConditionItem{Field: domain.FieldEmail, Operator: domain.OperatorEquals, Value: "authoritative.alice@example.com"}},
			{Item: &domain.ConditionItem{Field: domain.FieldUsername, Operator: domain.OperatorEquals, Value: "auth_alice"}},
		},
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, userResolver, nil, logger.NewNop())

	event := Event{
		ID:         "evt-reg-authoritative",
		Type:       EventUserRegistered,
		UserID:     "usr-authoritative-1",
		OccurredAt: time.Now().UTC(),
	}

	res, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected HandleEvent error: %v", err)
	}

	if len(res.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res.Results))
	}
	if res.Results[0].Status != StatusExecuted {
		t.Fatalf("expected StatusExecuted, got %s (err: %s)", res.Results[0].Status, res.Results[0].Error)
	}

	// Verify that the notification used the authoritative email and username
	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 created notification, got %d", len(directRepo.created))
	}
	notif := directRepo.created[0]
	if notif.RecipientEmail != "authoritative.alice@example.com" {
		t.Errorf("expected authoritative email 'authoritative.alice@example.com', got %q", notif.RecipientEmail)
	}
	if notif.RecipientName != "auth_alice" {
		t.Errorf("expected authoritative username 'auth_alice', got %q", notif.RecipientName)
	}
	if notif.ExternalUserID != "usr-authoritative-1" {
		t.Errorf("expected ExternalUserID 'usr-authoritative-1', got %q", notif.ExternalUserID)
	}
}

func TestEngine_AuthoritativeUserResolution_Failures(t *testing.T) {
	ruleRepo := &mockRuleRepo{
		rules: []*domain.AutomationRule{
			createTestRule("rule-1", "Active User Rule", "tpl-1", true, domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
				},
			}, nil),
		},
	}
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

	tests := []struct {
		name        string
		resolverErr error
		expectedErr string
	}{
		{
			name:        "User Not Found",
			resolverErr: userclient.ErrNotFound,
			expectedErr: "user not found",
		},
		{
			name:        "Unauthorized 401",
			resolverErr: userclient.ErrUnauthorized,
			expectedErr: "unauthorized",
		},
		{
			name:        "Forbidden 403",
			resolverErr: userclient.ErrForbidden,
			expectedErr: "forbidden",
		},
		{
			name:        "Server Error 500",
			resolverErr: userclient.ErrServer,
			expectedErr: "server error",
		},
		{
			name:        "Transport / Timeout Error",
			resolverErr: context.DeadlineExceeded,
			expectedErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directRepo := &mockDirectRepo{}
			execRepo := &mockExecRepo{directRepo: directRepo}

			failingResolver := &mockFailingUserResolver{err: tt.resolverErr}
			engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, failingResolver, nil, logger.NewNop())

			event := Event{
				ID:         "evt-fail-test",
				Type:       EventUserRegistered,
				UserID:     "non-existent-or-failing-user",
				OccurredAt: time.Now().UTC(),
			}

			res, err := engine.HandleEvent(context.Background(), event)
			if err != nil {
				t.Fatalf("unexpected HandleEvent error: %v", err)
			}

			if len(res.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(res.Results))
			}
			if res.Results[0].Status != StatusFailed {
				t.Errorf("expected StatusFailed, got %s", res.Results[0].Status)
			}
			if !strings.Contains(strings.ToLower(res.Results[0].Error), strings.ToLower(tt.expectedErr)) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, res.Results[0].Error)
			}
			if len(directRepo.created) != 0 {
				t.Errorf("expected 0 notifications created on user resolution failure, got %d", len(directRepo.created))
			}
		})
	}
}

type mockFailingUserResolver struct {
	err error
}

func (m *mockFailingUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	return nil, m.err
}

func TestEngine_InactivityEvaluation_AuthoritativeLastActivityAtSemantics(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	// Rule: Inactive for >= 30 days
	rule := createTestRule("rule-inactivity-30d", "Inactive 30 Days", "tpl-inactivity", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{Item: &domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     "days",
			}},
		},
	}, nil)
	rule.Config.Trigger = domain.TriggerUserInactive

	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{rule}}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-inactivity": {
				ID:           "tpl-inactivity",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "We miss you",
				HTMLBody:     "<p>Come back!</p>",
			},
		},
	}

	activity35DaysAgo := now.AddDate(0, 0, -35)
	activity30DaysAgo := now.AddDate(0, 0, -30)
	activity10DaysAgo := now.AddDate(0, 0, -10)
	reg60DaysAgo := now.AddDate(0, 0, -60)

	users := []userclient.User{
		// 1. LastActivityAt == nil -> NOT inactive (even if registration date was 60 days ago)
		{
			ID:               "u-nil-activity",
			Username:         "bob_nil",
			Email:            "bob_nil@example.com",
			RegistrationDate: reg60DaysAgo,
			LastActivityAt:   nil,
		},
		// 2. LastActivityAt 35 days ago -> Inactive (>= 30 days)
		{
			ID:               "u-35d-activity",
			Username:         "alice_35d",
			Email:            "alice_35d@example.com",
			RegistrationDate: reg60DaysAgo,
			LastActivityAt:   &activity35DaysAgo,
		},
		// 3. LastActivityAt exactly 30 days ago -> Inactive (boundary)
		{
			ID:               "u-30d-activity",
			Username:         "charlie_30d",
			Email:            "charlie_30d@example.com",
			RegistrationDate: reg60DaysAgo,
			LastActivityAt:   &activity30DaysAgo,
		},
		// 4. LastActivityAt 10 days ago -> NOT inactive (< 30 days)
		{
			ID:               "u-10d-activity",
			Username:         "david_10d",
			Email:            "david_10d@example.com",
			RegistrationDate: reg60DaysAgo,
			LastActivityAt:   &activity10DaysAgo,
		},
	}

	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}
	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, nil, logger.NewNop())

	userLister := &mockStaticUserLister{users: users}
	summary, err := engine.EvaluateInactivity(context.Background(), userLister, now, 50)
	if err != nil {
		t.Fatalf("unexpected EvaluateInactivity error: %v", err)
	}

	if summary.TotalUsersEvaluated != 4 {
		t.Errorf("expected 4 evaluated users, got %d", summary.TotalUsersEvaluated)
	}
	// Exactly 2 users should match inactivity (35 days ago and 30 days ago)
	if summary.ExecutedCount != 2 {
		t.Errorf("expected 2 executed rules for inactive users, got %d", summary.ExecutedCount)
	}
	if summary.SkippedNotMatchedCount != 2 {
		t.Errorf("expected 2 skipped non-matching users, got %d", summary.SkippedNotMatchedCount)
	}

	// Verify created notifications
	if len(directRepo.created) != 2 {
		t.Fatalf("expected 2 created notifications, got %d", len(directRepo.created))
	}
	emails := map[string]bool{
		directRepo.created[0].RecipientEmail: true,
		directRepo.created[1].RecipientEmail: true,
	}
	if !emails["alice_35d@example.com"] || !emails["charlie_30d@example.com"] {
		t.Errorf("expected notifications sent to alice_35d and charlie_30d, got: %+v", directRepo.created)
	}
}

type mockStaticUserLister struct {
	users []userclient.User
}

func (m *mockStaticUserLister) GetUsers(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
	if page > 1 {
		return &userclient.PagedUsers{Items: []userclient.User{}, HasNextPage: false}, nil
	}
	return &userclient.PagedUsers{
		Items:       m.users,
		TotalCount:  len(m.users),
		Page:        1,
		PageSize:    pageSize,
		HasNextPage: false,
	}, nil
}

func TestEngine_AuthoritativeUserPrecedenceOverEventPayload(t *testing.T) {
	ruleRepo := &mockRuleRepo{}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-prec": {
				ID:           "tpl-prec",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}",
				HTMLBody:     "<p>Email: {{email}}, Role: {{role}}, Language: {{language}}</p>",
			},
		},
	}

	// Rule matches role = "moderator" (authoritative) and isBlocked = false (authoritative)
	rule := createTestRule("rule-prec", "Precedence Check", "tpl-prec", true, domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldRole,
					Operator: domain.OperatorEquals,
					Value:    "moderator",
				},
			},
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldIsBlocked,
					Operator: domain.OperatorEquals,
					Value:    false,
				},
			},
		},
	}, nil)
	ruleRepo.rules = []*domain.AutomationRule{rule}

	directRepo := &mockDirectRepo{}
	execRepo := &mockExecRepo{directRepo: directRepo}

	// Authoritative user resolver returns: role="moderator", isBlocked=false, email="auth@example.com", username="authuser"
	userResolver := &mockUserResolver{
		users: map[string]*userclient.User{
			"u-prec-1": {
				ID:        "u-prec-1",
				Username:  "authuser",
				Email:     "auth@example.com",
				Role:      "moderator",
				Language:  "en",
				IsActive:  true,
				IsBlocked: false,
			},
		},
	}

	engine := NewEngine(ruleRepo, templateRepo, directRepo, execRepo, userResolver, nil, logger.NewNop())

	// Event payload maliciously/stale-ly claims: role="player", isBlocked=true, email="fake@example.com", username="fakeuser"
	// but also provides event-specific field "orderId" = "order-123"
	event := Event{
		ID:     "evt-prec-1",
		Type:   EventUserRegistered,
		UserID: "u-prec-1",
		Data: map[string]any{
			"role":      "player", // conflicting stale data
			"isBlocked": true,     // conflicting stale data
			"email":     "fake@example.com",
			"username":  "fakeuser",
			"orderId":   "order-123", // custom event-specific data
		},
	}

	result, err := engine.HandleEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected HandleEvent error: %v", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Status != StatusExecuted {
		t.Fatalf("expected rule to execute based on authoritative role=moderator and isBlocked=false, got status %s (err: %s)", result.Results[0].Status, result.Results[0].Error)
	}

	if len(directRepo.created) != 1 {
		t.Fatalf("expected 1 direct notification created, got %d", len(directRepo.created))
	}

	notif := directRepo.created[0]
	// Verify authoritative recipient and user fields were used
	if notif.RecipientEmail != "auth@example.com" {
		t.Errorf("expected recipient email to be authoritative 'auth@example.com', got %q", notif.RecipientEmail)
	}
	if notif.RecipientName != "authuser" {
		t.Errorf("expected recipient name to be authoritative 'authuser', got %q", notif.RecipientName)
	}

	// Verify custom event payload was preserved
	var payloadData map[string]any
	if err := json.Unmarshal(notif.Payload, &payloadData); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payloadData["orderId"] != "order-123" {
		t.Errorf("expected payload to contain custom event data 'orderId' = 'order-123', got %v", payloadData["orderId"])
	}
	if payloadData["role"] != "moderator" {
		t.Errorf("expected payload 'role' to be authoritative 'moderator', got %v", payloadData["role"])
	}
}
