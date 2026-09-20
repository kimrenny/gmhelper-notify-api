package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/automation"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/postgres"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
	"github.com/google/uuid"
)

func TestIntegration_AutomationEventEndpoint_PostgreSQL(t *testing.T) {
	db := getIntegrationDB(t)
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log, _ := logger.NewLogger("error")

	// Repositories backed by real PostgreSQL
	activityRepo := postgres.NewActivityLogRepository(db)
	auditService := audit.NewService(activityRepo)
	templateRepo := postgres.NewEmailTemplateRepository(db)
	directRepo := postgres.NewDirectNotificationRepository(db)
	ruleRepo := postgres.NewAutomationRuleRepository(db)
	execRepo := postgres.NewAutomationExecutionRepository(db)

	engine := automation.NewEngine(ruleRepo, templateRepo, directRepo, execRepo, nil, auditService, log)
	eventHandler := handlers.NewAutomationEventHandler(engine, log)

	jwtVerifier := auth.MustNewJWTVerifier(intTestSecret, intTestIssuer, intTestAudience)
	authMiddleware := middleware.AdminAuth(jwtVerifier, log)

	router := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, eventHandler, authMiddleware)

	// Create test template in DB
	tplKey := "e2e_evt_tpl_" + uuid.NewString()[:8]
	tpl := &domain.EmailTemplate{
		ID:           uuid.NewString(),
		TemplateKey:  tplKey,
		Name:         "E2E Event Template",
		TemplateType: domain.TemplateTypeAutomation,
		Subject:      "Welcome {{username}}!",
		HTMLBody:     "<p>Welcome {{username}} from {{source}}</p>",
		Locale:       "en",
		Status:       domain.TemplateStatusActive,
		Version:      1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, tpl); err != nil {
		t.Fatalf("failed to create template: %v", err)
	}

	eventID := uuid.NewString()
	userID := "user-alex-" + uuid.NewString()[:8]
	ruleID := uuid.NewString()

	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCleanup()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM automation_executions WHERE rule_id = $1", ruleID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM direct_notifications WHERE external_user_id = $1", userID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM automation_rules WHERE id = $1", ruleID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM email_templates WHERE id = $1", tpl.ID)
	})

	// Create test automation rule in DB
	cooldown := 10
	rule := &domain.AutomationRule{
		ID: ruleID,

		Name:       "E2E Event Rule",
		TemplateID: tpl.ID,
		Enabled:    true,
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserRegistered,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(3),
				MinuteUTC: intPtr(0),
			},
			Conditions: domain.ConditionGroup{
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
			},
			Action: domain.ActionConfig{
				Type:         domain.ActionTypeSendEmail,
				CooldownDays: &cooldown,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := ruleRepo.Create(ctx, rule); err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	serviceToken, err := auth.GenerateToken(intTestSecret, intTestIssuer, intTestAudience, "gmhelper-api-svc", "service", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate service token: %v", err)
	}

	eventPayload := map[string]any{
		"id":         eventID,
		"type":       "user.registered",
		"userId":     userID,
		"occurredAt": time.Now().UTC().Format(time.RFC3339),
		"user": userclient.User{
			ID:       userID,
			Username: "alex",
			Email:    "alex@example.com",
			Role:     "User",
			Language: "EN",
			IsActive: true,
		},
		"data": map[string]any{
			"source": "registration_flow",
		},
	}
	payloadBytes, _ := json.Marshal(eventPayload)

	// 1. Initial event dispatch -> 200 OK & StatusExecuted
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(payloadBytes))
	req1.Header.Set("Authorization", "Bearer "+serviceToken)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on first dispatch, got %d (body: %s)", rec1.Code, rec1.Body.String())
	}

	var res1 automation.EventExecutionResult
	if err := json.Unmarshal(rec1.Body.Bytes(), &res1); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	var targetRuleRes *automation.RuleExecutionResult
	for _, r := range res1.Results {
		if r.RuleID == rule.ID {
			targetRuleRes = &r
			break
		}
	}
	if targetRuleRes == nil || targetRuleRes.Status != automation.StatusExecuted {
		t.Fatalf("expected rule to be executed, got: %+v", res1.Results)
	}
	if targetRuleRes.NotificationID == nil {
		t.Fatalf("expected NotificationID to be present")
	}

	// Verify direct notification exists in PostgreSQL
	notif, err := directRepo.GetByID(ctx, *targetRuleRes.NotificationID)
	if err != nil {
		t.Fatalf("failed to retrieve direct notification from DB: %v", err)
	}
	if notif.RecipientEmail != "alex@example.com" || notif.NotificationType != domain.NotificationTypeAutomation {
		t.Errorf("unexpected direct notification fields: %+v", notif)
	}

	// Clean up created notification immediately so concurrent background worker tests are isolated
	_, _ = db.ExecContext(ctx, "DELETE FROM direct_notifications WHERE id = $1", *targetRuleRes.NotificationID)

	// 2. Duplicate dispatch with identical event ID -> 200 OK & StatusSkippedDuplicate

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(payloadBytes))
	req2.Header.Set("Authorization", "Bearer "+serviceToken)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on duplicate dispatch, got %d", rec2.Code)
	}

	var res2 automation.EventExecutionResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &res2); err != nil {
		t.Fatalf("failed to decode duplicate response: %v", err)
	}
	var dupRuleRes *automation.RuleExecutionResult
	for _, r := range res2.Results {
		if r.RuleID == rule.ID {
			dupRuleRes = &r
			break
		}
	}
	if dupRuleRes == nil || dupRuleRes.Status != automation.StatusSkippedDuplicate {
		t.Errorf("expected duplicate rule status skipped_duplicate, got: %+v", res2.Results)
	}
}
