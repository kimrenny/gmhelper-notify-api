package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/automation"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

type mockRuleRepoForHandler struct {
	rules   []*domain.AutomationRule
	listErr error
}

func (m *mockRuleRepoForHandler) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	for _, r := range m.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockRuleRepoForHandler) Create(ctx context.Context, rule *domain.AutomationRule) error {
	return nil
}
func (m *mockRuleRepoForHandler) Update(ctx context.Context, rule *domain.AutomationRule) error {
	return nil
}
func (m *mockRuleRepoForHandler) UpdateEvaluationTimes(ctx context.Context, id string, lastEvaluatedAt, nextEvaluationAt *time.Time) error {
	return nil
}
func (m *mockRuleRepoForHandler) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockRuleRepoForHandler) List(ctx context.Context) ([]*domain.AutomationRule, error) {
	return m.rules, nil
}
func (m *mockRuleRepoForHandler) ListEnabled(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.rules, nil
}

type mockExecRepoForHandler struct {
	directRepo *mockDirectRepoForHandler
	status     string
	err        error
}

func (m *mockExecRepoForHandler) RecordExecution(ctx context.Context, exec *domain.AutomationExecution) error {
	return nil
}

func (m *mockExecRepoForHandler) HasExecution(ctx context.Context, ruleID, eventID string) (bool, error) {
	return false, nil
}

func (m *mockExecRepoForHandler) GetLastSuccessfulExecution(ctx context.Context, ruleID, recipientEmail string, externalUserID *string) (*domain.AutomationExecution, error) {
	return nil, nil
}

func (m *mockExecRepoForHandler) ListByRuleID(ctx context.Context, ruleID string, limit, offset int) ([]*domain.AutomationExecution, int, error) {
	return []*domain.AutomationExecution{}, 0, nil
}

func (m *mockExecRepoForHandler) ExecuteRuleAtomic(
	ctx context.Context,
	ruleID string,
	eventID string,
	recipientEmail string,
	externalUserID *string,
	cooldownDays *int,
	occurredAt time.Time,
	notif *domain.DirectNotification,
	exec *domain.AutomationExecution,
) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.directRepo != nil && notif != nil {
		_ = m.directRepo.Create(ctx, notif)
	}
	if m.status != "" {
		return m.status, nil
	}
	return "success", nil
}

type mockTemplateRepoForHandler struct {
	templates map[string]*domain.EmailTemplate
}

func (m *mockTemplateRepoForHandler) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if tpl, ok := m.templates[id]; ok {
		return tpl, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockTemplateRepoForHandler) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	for _, tpl := range m.templates {
		if tpl.TemplateKey == key {
			return tpl, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockTemplateRepoForHandler) Create(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}
func (m *mockTemplateRepoForHandler) Update(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}
func (m *mockTemplateRepoForHandler) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockTemplateRepoForHandler) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

func TestAutomationEventHandler_HandleEvent_Validation(t *testing.T) {
	log, _ := logger.NewLogger("error")
	tplRepo := &mockTemplateRepoForHandler{templates: make(map[string]*domain.EmailTemplate)}
	engine := automation.NewEngine(&mockRuleRepoForHandler{}, tplRepo, nil, &mockExecRepoForHandler{}, nil, nil, log)
	handler := NewAutomationEventHandler(engine, log)

	now := time.Now().UTC()

	tests := []struct {
		name       string
		body       any
		rawBody    *string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "Nil Body",
			rawBody:    nil,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "Empty Body",
			rawBody:    func() *string { s := ""; return &s }(),
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "Malformed JSON",
			rawBody:    func() *string { s := "{invalid json"; return &s }(),
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing Event ID",
			body: map[string]any{
				"id":         "",
				"type":       "user.registered",
				"userId":     "user-1",
				"occurredAt": now.Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing Event Type",
			body: map[string]any{
				"id":         "evt-1",
				"type":       "   ",
				"userId":     "user-1",
				"occurredAt": now.Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing User ID",
			body: map[string]any{
				"id":         "evt-1",
				"type":       "user.registered",
				"userId":     "",
				"occurredAt": now.Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing occurredAt",
			body: map[string]any{
				"id":     "evt-1",
				"type":   "user.registered",
				"userId": "user-1",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Invalid occurredAt format",
			body: map[string]any{
				"id":         "evt-1",
				"type":       "user.registered",
				"userId":     "user-1",
				"occurredAt": "invalid-timestamp",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.rawBody != nil {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader([]byte(*tc.rawBody)))
			} else if tc.body != nil {
				b, _ := json.Marshal(tc.body)
				req = httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(b))
			} else {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", nil)
			}
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			handler.HandleEvent(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAutomationEventHandler_HandleEvent_SuccessAndExecution(t *testing.T) {
	log, _ := logger.NewLogger("info")
	rule := &domain.AutomationRule{
		ID:         "rule-100",
		Name:       "Welcome Automation",
		TemplateID: "tpl-welcome",
		Enabled:    true,
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserRegistered,
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
				Type: domain.ActionTypeSendEmail,
			},
		},
	}
	ruleRepo := &mockRuleRepoForHandler{rules: []*domain.AutomationRule{rule}}
	tplRepo := &mockTemplateRepoForHandler{
		templates: map[string]*domain.EmailTemplate{
			"tpl-welcome": {
				ID:           "tpl-welcome",
				TemplateKey:  "welcome_tpl",
				Name:         "Welcome",
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}!",
				HTMLBody:     "<p>Welcome {{username}}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
		},
	}
	execRepo := &mockExecRepoForHandler{status: "success"}

	engine := automation.NewEngine(ruleRepo, tplRepo, nil, execRepo, nil, nil, log)
	handler := NewAutomationEventHandler(engine, log)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "event-auto-1",
		Type:       "user.registered",
		UserID:     "user-alex-1",
		OccurredAt: &now,
		User: &userclient.User{
			ID:       "user-alex-1",
			Username: "alex",
			Email:    "alex@example.com",
			Role:     "User",
			Language: "EN",
			IsActive: true,
		},
		Data: map[string]any{
			"source": "mobile_app",
		},
	}

	bodyBytes, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleEvent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res automation.EventExecutionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.EventID != "event-auto-1" || res.EventType != "user.registered" {
		t.Errorf("unexpected event result metadata: %+v", res)
	}

	if len(res.Results) != 1 {
		t.Fatalf("expected 1 rule execution result, got %d", len(res.Results))
	}

	if res.Results[0].RuleID != "rule-100" || res.Results[0].Status != automation.StatusExecuted {
		t.Errorf("unexpected rule outcome: %+v", res.Results[0])
	}
}

func TestAutomationEventHandler_HandleEvent_IdempotencyAndCooldownSemantics(t *testing.T) {
	log, _ := logger.NewLogger("info")
	rule := &domain.AutomationRule{
		ID:         "rule-duplicate",
		Name:       "Duplicate Test Rule",
		TemplateID: "tpl-welcome",
		Enabled:    true,
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserRegistered,
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
				Type: domain.ActionTypeSendEmail,
			},
		},
	}
	ruleRepo := &mockRuleRepoForHandler{rules: []*domain.AutomationRule{rule}}
	tplRepo := &mockTemplateRepoForHandler{
		templates: map[string]*domain.EmailTemplate{
			"tpl-welcome": {
				ID:           "tpl-welcome",
				TemplateKey:  "welcome_tpl",
				Name:         "Welcome",
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Welcome {{username}}!",
				HTMLBody:     "<p>Welcome {{username}}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
		},
	}

	// 1. Test skipped_duplicate response
	execRepoDup := &mockExecRepoForHandler{status: "skipped_duplicate"}
	engineDup := automation.NewEngine(ruleRepo, tplRepo, nil, execRepoDup, nil, nil, log)
	handlerDup := NewAutomationEventHandler(engineDup, log)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "event-dup-1",
		Type:       "user.registered",
		UserID:     "user-alex-1",
		OccurredAt: &now,
		User: &userclient.User{
			ID:       "user-alex-1",
			Username: "alex",
			Email:    "alex@example.com",
			IsActive: true,
		},
	}
	bodyBytes, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	handlerDup.HandleEvent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resDup automation.EventExecutionResult
	_ = json.Unmarshal(rec.Body.Bytes(), &resDup)
	if len(resDup.Results) != 1 || resDup.Results[0].Status != automation.StatusSkippedDuplicate {
		t.Errorf("expected skipped_duplicate, got: %+v", resDup)
	}

	// 2. Test skipped_cooldown response
	execRepoCooldown := &mockExecRepoForHandler{status: "skipped_cooldown"}
	engineCooldown := automation.NewEngine(ruleRepo, tplRepo, nil, execRepoCooldown, nil, nil, log)
	handlerCooldown := NewAutomationEventHandler(engineCooldown, log)

	recCool := httptest.NewRecorder()
	reqCool := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(bodyBytes))
	handlerCooldown.HandleEvent(recCool, reqCool)

	if recCool.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recCool.Code)
	}

	var resCool automation.EventExecutionResult
	_ = json.Unmarshal(recCool.Body.Bytes(), &resCool)
	if len(resCool.Results) != 1 || resCool.Results[0].Status != automation.StatusSkippedCooldown {
		t.Errorf("expected skipped_cooldown, got: %+v", resCool)
	}
}

func TestAutomationEventHandler_HandleEvent_EngineErrorPropagation(t *testing.T) {
	log, _ := logger.NewLogger("error")
	ruleRepo := &mockRuleRepoForHandler{listErr: errors.New("db connection failure")}
	engine := automation.NewEngine(ruleRepo, nil, nil, nil, nil, nil, log)
	handler := NewAutomationEventHandler(engine, log)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "event-err-1",
		Type:       "user.registered",
		UserID:     "user-1",
		OccurredAt: &now,
	}
	b, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(b))
	rec := httptest.NewRecorder()

	handler.HandleEvent(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 on engine error, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to valid JSON error response: %v", err)
	}

	if errResp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR error code, got %s", errResp.Error.Code)
	}
	// Verify internal details like "db connection failure" are not leaked to the client
	if errResp.Error.Message == "db connection failure" {
		t.Errorf("expected masked error message, got raw db error: %s", errResp.Error.Message)
	}
}

func TestAutomationEventHandler_HandleEvent_EngineNilSafety(t *testing.T) {
	log, _ := logger.NewLogger("error")
	handler := NewAutomationEventHandler(nil, log)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "event-nil-1",
		Type:       "user.registered",
		UserID:     "user-1",
		OccurredAt: &now,
	}
	b, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(b))
	rec := httptest.NewRecorder()

	handler.HandleEvent(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when engine is nil, got %d", rec.Code)
	}
}

func TestAutomationEventHandler_ServiceAuthIntegration(t *testing.T) {
	log, _ := logger.NewLogger("info")
	secret := "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
	issuer := "gmhelper-api"
	audience := "gmhelper-notify-api"

	tplRepo := &mockTemplateRepoForHandler{templates: make(map[string]*domain.EmailTemplate)}
	engine := automation.NewEngine(&mockRuleRepoForHandler{}, tplRepo, nil, &mockExecRepoForHandler{}, nil, nil, log)
	eventHandler := NewAutomationEventHandler(engine, log)

	verifier := auth.MustNewJWTVerifier(secret, issuer, audience)
	serviceAuthMw := middleware.ServiceAuth(verifier, log)

	mux := http.NewServeMux()
	mux.Handle("POST /internal/automation/events", serviceAuthMw(http.HandlerFunc(eventHandler.HandleEvent)))

	serviceToken, _ := auth.GenerateToken(secret, issuer, audience, "svc-1", "service", 15*time.Minute)
	userToken, _ := auth.GenerateToken(secret, issuer, audience, "usr-1", "user", 15*time.Minute)
	adminToken, _ := auth.GenerateToken(secret, issuer, audience, "adm-1", "admin", 15*time.Minute)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "event-int-1",
		Type:       "user.registered",
		UserID:     "user-1",
		OccurredAt: &now,
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	// 1. Unauthenticated -> 401
	req1 := httptest.NewRequest(http.MethodPost, "/internal/automation/events", bytes.NewReader(bodyBytes))
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing token, got %d", rec1.Code)
	}

	// 2. User role -> 403
	req2 := httptest.NewRequest(http.MethodPost, "/internal/automation/events", bytes.NewReader(bodyBytes))
	req2.Header.Set("Authorization", "Bearer "+userToken)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for role 'user', got %d", rec2.Code)
	}

	// 3. Admin role -> 403 (service-only)
	req3 := httptest.NewRequest(http.MethodPost, "/internal/automation/events", bytes.NewReader(bodyBytes))
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for role 'admin', got %d", rec3.Code)
	}

	// 4. Service role -> 200 OK
	req4 := httptest.NewRequest(http.MethodPost, "/internal/automation/events", bytes.NewReader(bodyBytes))
	req4.Header.Set("Authorization", "Bearer "+serviceToken)
	rec4 := httptest.NewRecorder()
	mux.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Errorf("expected 200 OK for role 'service', got %d (body: %s)", rec4.Code, rec4.Body.String())
	}
}

type mockDirectRepoForHandler struct {
	created []*domain.DirectNotification
}

func (m *mockDirectRepoForHandler) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	for _, n := range m.created {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockDirectRepoForHandler) Create(ctx context.Context, notification *domain.DirectNotification) error {
	m.created = append(m.created, notification)
	return nil
}

func (m *mockDirectRepoForHandler) CreateWithInitialAttempt(ctx context.Context, notification *domain.DirectNotification, attempt *domain.DeliveryAttempt) error {
	m.created = append(m.created, notification)
	return nil
}

func (m *mockDirectRepoForHandler) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	return nil, nil
}

func (m *mockDirectRepoForHandler) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	return nil, nil
}

func (m *mockDirectRepoForHandler) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	return 0, nil
}

func (m *mockDirectRepoForHandler) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	return nil
}

type mockUserResolverForHandler struct {
	users map[string]*userclient.User
}

func (m *mockUserResolverForHandler) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, userclient.ErrNotFound
}

func TestAutomationEventHandler_EndToEnd_SupportedLifecycleEvents(t *testing.T) {
	log, _ := logger.NewLogger("info")
	secret := "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
	issuer := "gmhelper-api"
	audience := "gmhelper-notify-api"

	events := []struct {
		eventType string
		ruleID    string
		tplID     string
	}{
		{eventType: "user.registered", ruleID: "rule-reg", tplID: "tpl-reg"},
		{eventType: "email.confirmed", ruleID: "rule-emc", tplID: "tpl-emc"},
		{eventType: "password.changed", ruleID: "rule-pwd", tplID: "tpl-pwd"},
		{eventType: "user.blocked", ruleID: "rule-blk", tplID: "tpl-blk"},
		{eventType: "user.unblocked", ruleID: "rule-unb", tplID: "tpl-unb"},
		{eventType: "user.language_changed", ruleID: "rule-lng", tplID: "tpl-lng"},
	}

	for _, tc := range events {
		t.Run(tc.eventType, func(t *testing.T) {
			ruleRepo := &mockRuleRepoForHandler{
				rules: []*domain.AutomationRule{
					{
						ID:         tc.ruleID,
						Name:       tc.eventType + " Rule",
						TemplateID: tc.tplID,
						Enabled:    true,
						Config: domain.AutomationRuleConfig{
							Version: 1,
							Trigger: tc.eventType,
							Conditions: domain.ConditionGroup{
								Operator: domain.GroupOperatorAll,
								Conditions: []domain.ConditionNode{
									{
										Item: &domain.ConditionItem{
											Field:    domain.FieldRole,
											Operator: domain.OperatorEquals,
											Value:    "User",
										},
									},
								},
							},
							Action: domain.ActionConfig{
								Type: domain.ActionTypeSendEmail,
							},
						},
					},
				},
			}

			tplRepo := &mockTemplateRepoForHandler{
				templates: map[string]*domain.EmailTemplate{
					tc.tplID: {
						ID:            tc.tplID,
						Status:        domain.TemplateStatusActive,
						TemplateType:  domain.TemplateTypeAutomation,
						Subject:       "Event " + tc.eventType + " for {{username}}",
						HTMLBody:      "<p>Hello {{username}}, event {{eventType}} occurred.</p>",
						PlainTextBody: "Hello {{username}}, event {{eventType}} occurred.",
					},
				},
			}

			directRepo := &mockDirectRepoForHandler{}
			execRepo := &mockExecRepoForHandler{directRepo: directRepo}
			userResolver := &mockUserResolverForHandler{
				users: map[string]*userclient.User{
					"usr-101": {
						ID:        "usr-101",
						Username:  "alex_authoritative",
						Email:     "authoritative.alex@example.com",
						Role:      "User",
						Language:  "EN",
						IsActive:  true,
						IsBlocked: false,
					},
				},
			}

			engine := automation.NewEngine(ruleRepo, tplRepo, directRepo, execRepo, userResolver, nil, log)
			eventHandler := NewAutomationEventHandler(engine, log)

			verifier := auth.MustNewJWTVerifier(secret, issuer, audience)
			serviceAuthMw := middleware.ServiceAuth(verifier, log)

			mux := http.NewServeMux()
			mux.Handle("POST /api/v1/internal/automation/events", serviceAuthMw(http.HandlerFunc(eventHandler.HandleEvent)))

			serviceToken, _ := auth.GenerateToken(secret, issuer, audience, "gmhelper-api", "service", 15*time.Minute)

			now := time.Now().UTC()
			reqPayload := AutomationEventRequest{
				ID:         "evt-e2e-" + tc.eventType,
				Type:       tc.eventType,
				UserID:     "usr-101",
				OccurredAt: &now,
			}
			bodyBytes, _ := json.Marshal(reqPayload)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(bodyBytes))
			req.Header.Set("Authorization", "Bearer "+serviceToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
			}

			var execResult automation.EventExecutionResult
			if err := json.Unmarshal(rec.Body.Bytes(), &execResult); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if len(execResult.Results) != 1 {
				t.Fatalf("expected 1 rule execution result, got %d", len(execResult.Results))
			}
			if execResult.Results[0].Status != automation.StatusExecuted {
				t.Fatalf("expected StatusExecuted, got %s (err: %s)", execResult.Results[0].Status, execResult.Results[0].Error)
			}

			// Verify direct notification created with authoritative email
			if len(directRepo.created) != 1 {
				t.Fatalf("expected 1 notification created, got %d", len(directRepo.created))
			}
			notif := directRepo.created[0]
			if notif.RecipientEmail != "authoritative.alex@example.com" {
				t.Errorf("expected authoritative recipient email 'authoritative.alex@example.com', got %q", notif.RecipientEmail)
			}
			if notif.RecipientName != "alex_authoritative" {
				t.Errorf("expected authoritative recipient name 'alex_authoritative', got %q", notif.RecipientName)
			}
		})
	}
}

func TestAutomationEventHandler_EndToEnd_UserInactiveTriggerIgnored(t *testing.T) {
	log, _ := logger.NewLogger("info")
	secret := "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
	issuer := "gmhelper-api"
	audience := "gmhelper-notify-api"

	ruleRepo := &mockRuleRepoForHandler{
		rules: []*domain.AutomationRule{
			{
				ID:         "rule-inactive",
				Name:       "Inactivity Rule",
				TemplateID: "tpl-inact",
				Enabled:    true,
				Config: domain.AutomationRuleConfig{
					Version: 1,
					Trigger: domain.TriggerUserInactive,
					Conditions: domain.ConditionGroup{
						Operator: domain.GroupOperatorAll,
					},
					Action: domain.ActionConfig{
						Type: domain.ActionTypeSendEmail,
					},
				},
			},
		},
	}

	tplRepo := &mockTemplateRepoForHandler{
		templates: map[string]*domain.EmailTemplate{
			"tpl-inact": {
				ID:           "tpl-inact",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Miss you",
				HTMLBody:     "<p>Miss you</p>",
			},
		},
	}

	directRepo := &mockDirectRepoForHandler{}
	execRepo := &mockExecRepoForHandler{directRepo: directRepo}
	userResolver := &mockUserResolverForHandler{
		users: map[string]*userclient.User{
			"usr-101": {
				ID:       "usr-101",
				Username: "alex",
				Email:    "alex@example.com",
				IsActive: true,
			},
		},
	}

	engine := automation.NewEngine(ruleRepo, tplRepo, directRepo, execRepo, userResolver, nil, log)
	eventHandler := NewAutomationEventHandler(engine, log)

	verifier := auth.MustNewJWTVerifier(secret, issuer, audience)
	serviceAuthMw := middleware.ServiceAuth(verifier, log)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/internal/automation/events", serviceAuthMw(http.HandlerFunc(eventHandler.HandleEvent)))

	serviceToken, _ := auth.GenerateToken(secret, issuer, audience, "gmhelper-api", "service", 15*time.Minute)

	now := time.Now().UTC()
	reqPayload := AutomationEventRequest{
		ID:         "evt-inact-1",
		Type:       domain.TriggerUserInactive,
		UserID:     "usr-101",
		OccurredAt: &now,
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/automation/events", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+serviceToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var execResult automation.EventExecutionResult
	_ = json.Unmarshal(rec.Body.Bytes(), &execResult)

	// user.inactive must be ignored by the event ingestion path (0 rules executed)
	if len(execResult.Results) != 0 {
		t.Errorf("expected 0 executed rules for user.inactive via event path, got %d", len(execResult.Results))
	}
	if len(directRepo.created) != 0 {
		t.Errorf("expected 0 notifications created, got %d", len(directRepo.created))
	}
}
