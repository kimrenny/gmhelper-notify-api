package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
)

func intPtr(i int) *int {
	return &i
}

func sampleConfig() domain.AutomationRuleConfig {
	return domain.AutomationRuleConfig{
		Version: 1,
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
			CooldownDays: intPtr(30),
		},
	}
}

type mockAutomationRepo struct {
	rules     map[string]*domain.AutomationRule
	createErr error
	updateErr error
	deleteErr error
	getErr    error
	listErr   error
	listEnErr error
}

func newMockAutomationRepo() *mockAutomationRepo {
	return &mockAutomationRepo{
		rules: make(map[string]*domain.AutomationRule),
	}
}

func (m *mockAutomationRepo) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	rule, ok := m.rules[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *rule
	return &cp, nil
}

func (m *mockAutomationRepo) Create(ctx context.Context, rule *domain.AutomationRule) error {
	if m.createErr != nil {
		return m.createErr
	}
	cp := *rule
	m.rules[rule.ID] = &cp
	return nil
}

func (m *mockAutomationRepo) Update(ctx context.Context, rule *domain.AutomationRule) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.rules[rule.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *rule
	m.rules[rule.ID] = &cp
	return nil
}

func (m *mockAutomationRepo) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.rules[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.rules, id)
	return nil
}

func (m *mockAutomationRepo) List(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	result := []*domain.AutomationRule{}
	for _, r := range m.rules {
		cp := *r
		result = append(result, &cp)
	}
	return result, nil
}

func (m *mockAutomationRepo) ListEnabled(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.listEnErr != nil {
		return nil, m.listEnErr
	}
	result := []*domain.AutomationRule{}
	for _, r := range m.rules {
		if r.Enabled {
			cp := *r
			result = append(result, &cp)
		}
	}
	return result, nil
}

type mockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
	getErr    error
}

func newMockTemplateRepo() *mockTemplateRepo {
	return &mockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
}

func (m *mockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	tpl, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *tpl
	return &cp, nil
}

func (m *mockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	return nil, nil
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *mockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *mockTemplateRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func TestService_List_Success(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	svc := NewService(autoRepo, tplRepo, nil)

	rule1 := &domain.AutomationRule{
		ID:         "r1",
		Name:       "Rule 1",
		TemplateID: "t1",
		Enabled:    true,
		Config:     sampleConfig(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	rule2 := &domain.AutomationRule{
		ID:         "r2",
		Name:       "Rule 2",
		TemplateID: "t2",
		Enabled:    false,
		Config:     sampleConfig(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = autoRepo.Create(context.Background(), rule1)
	_ = autoRepo.Create(context.Background(), rule2)

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(list))
	}
}

func TestService_GetByID(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	svc := NewService(autoRepo, tplRepo, nil)

	rule := &domain.AutomationRule{
		ID:         "r-100",
		Name:       "Welcome Rule",
		TemplateID: "t-100",
		Enabled:    true,
		Config:     sampleConfig(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = autoRepo.Create(context.Background(), rule)

	// 1. Success
	fetched, err := svc.GetByID(context.Background(), "r-100")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if fetched.Name != "Welcome Rule" {
		t.Errorf("expected name 'Welcome Rule', got '%s'", fetched.Name)
	}

	// 2. Empty ID -> ErrInvalidInput
	_, err = svc.GetByID(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// 3. Not Found -> ErrNotFound
	_, err = svc.GetByID(context.Background(), "non-existent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got %v", err)
	}
}

func TestService_Create(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["t-valid"] = &domain.EmailTemplate{
		ID:           "t-valid",
		Name:         "Valid Template",
		TemplateType: domain.TemplateTypeAutomation,
	}
	svc := NewService(autoRepo, tplRepo, nil)

	// 1. Success
	enabled := true
	cfg := sampleConfig()
	created, err := svc.Create(context.Background(), CreateInput{
		Name:       "New User Automation",
		TemplateID: "t-valid",
		Enabled:    &enabled,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if created.ID == "" || created.Name != "New User Automation" || !created.Enabled {
		t.Errorf("unexpected created rule: %+v", created)
	}

	// 2. Missing required fields -> ErrInvalidInput
	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "",
		TemplateID: "t-valid",
		Config:     cfg,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing name, got %v", err)
	}

	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "Rule",
		TemplateID: "",
		Config:     cfg,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing templateId, got %v", err)
	}

	// 3. Non-existent Template -> ErrTemplateNotFound
	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "Rule",
		TemplateID: "t-invalid-missing",
		Config:     cfg,
	})
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}

	// 4. Invalid Config (e.g. version 0) -> validation error
	invalidCfg := sampleConfig()
	invalidCfg.Version = 0
	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "Rule",
		TemplateID: "t-valid",
		Config:     invalidCfg,
	})
	if err == nil || !errors.Is(err, domain.ErrInvalidEntity) {
		t.Errorf("expected ErrInvalidEntity for invalid config, got %v", err)
	}

	// 5. Template Repo Database Error
	tplRepo.getErr = errors.New("db failure")
	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "Rule",
		TemplateID: "t-valid",
		Config:     cfg,
	})
	if err == nil || errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("expected underlying DB error, got %v", err)
	}
	tplRepo.getErr = nil

	// 6. Automation Repo Create Error
	autoRepo.createErr = errors.New("insert failed")
	_, err = svc.Create(context.Background(), CreateInput{
		Name:       "Rule",
		TemplateID: "t-valid",
		Config:     cfg,
	})
	if err == nil {
		t.Errorf("expected error on repo create failure, got nil")
	}
	autoRepo.createErr = nil
}

func TestService_Update(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["t-original"] = &domain.EmailTemplate{ID: "t-original", TemplateType: domain.TemplateTypeAutomation}
	tplRepo.templates["t-new"] = &domain.EmailTemplate{ID: "t-new", TemplateType: domain.TemplateTypeAutomation}

	svc := NewService(autoRepo, tplRepo, nil)

	existingRule := &domain.AutomationRule{
		ID:         "rule-to-edit",
		Name:       "Original Name",
		TemplateID: "t-original",
		Enabled:    true,
		Config:     sampleConfig(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = autoRepo.Create(context.Background(), existingRule)

	// 1. Success with mutable fields
	newName := "Updated Name"
	newTemplateID := "t-new"
	newEnabled := false
	newCfg := sampleConfig()
	newCfg.Schedule.Type = domain.ScheduleTypeIntervalHours
	newCfg.Schedule.HourUTC = nil
	newCfg.Schedule.MinuteUTC = nil
	newCfg.Schedule.IntervalHours = intPtr(48)

	updated, err := svc.Update(context.Background(), "rule-to-edit", UpdateInput{
		Name:       &newName,
		TemplateID: &newTemplateID,
		Enabled:    &newEnabled,
		Config:     &newCfg,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if updated.Name != newName || updated.TemplateID != newTemplateID || updated.Enabled != false {
		t.Errorf("unexpected updated fields: %+v", updated)
	}

	// 2. Empty ID -> ErrInvalidInput
	_, err = svc.Update(context.Background(), "   ", UpdateInput{Name: &newName})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// 3. Not Found -> ErrNotFound
	_, err = svc.Update(context.Background(), "rule-non-existent", UpdateInput{Name: &newName})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got %v", err)
	}

	// 4. Empty mutable field values -> ErrInvalidInput
	emptyStr := "  "
	_, err = svc.Update(context.Background(), "rule-to-edit", UpdateInput{Name: &emptyStr})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty name, got %v", err)
	}
	_, err = svc.Update(context.Background(), "rule-to-edit", UpdateInput{TemplateID: &emptyStr})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty templateId, got %v", err)
	}

	// 5. Updating to non-existent template -> ErrTemplateNotFound
	missingTpl := "t-missing-404"
	_, err = svc.Update(context.Background(), "rule-to-edit", UpdateInput{TemplateID: &missingTpl})
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}

	// 6. Template repo error during update
	tplRepo.getErr = errors.New("tpl db error")
	anotherTpl := "t-another"
	_, err = svc.Update(context.Background(), "rule-to-edit", UpdateInput{TemplateID: &anotherTpl})
	if err == nil || errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("expected tpl db error, got %v", err)
	}
	tplRepo.getErr = nil

	// 7. Repo update error
	autoRepo.updateErr = errors.New("update sql failed")
	validName := "Valid Name"
	_, err = svc.Update(context.Background(), "rule-to-edit", UpdateInput{Name: &validName})
	if err == nil {
		t.Errorf("expected repo update error, got nil")
	}
	autoRepo.updateErr = nil
}

func TestService_Delete(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	svc := NewService(autoRepo, tplRepo, nil)

	rule := &domain.AutomationRule{
		ID:         "rule-del",
		Name:       "Rule to delete",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), rule)

	// 1. Success
	if err := svc.Delete(context.Background(), "rule-del"); err != nil {
		t.Fatalf("expected successful delete, got %v", err)
	}
	if _, ok := autoRepo.rules["rule-del"]; ok {
		t.Errorf("rule still present in repo after delete")
	}

	// 2. Empty ID -> ErrInvalidInput
	if err := svc.Delete(context.Background(), "   "); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// 3. Not Found -> ErrNotFound
	if err := svc.Delete(context.Background(), "rule-del"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound on second delete, got %v", err)
	}

	// 4. Repo Delete Error
	_ = autoRepo.Create(context.Background(), rule)
	autoRepo.deleteErr = errors.New("delete error")
	if err := svc.Delete(context.Background(), "rule-del"); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected delete error, got %v", err)
	}
}

func TestService_NilRepo(t *testing.T) {
	svc := NewService(nil, nil, nil)

	if _, err := svc.List(context.Background()); err == nil {
		t.Errorf("expected error with nil repo on List")
	}
	if _, err := svc.GetByID(context.Background(), "1"); err == nil {
		t.Errorf("expected error with nil repo on GetByID")
	}
	if _, err := svc.Create(context.Background(), CreateInput{}); err == nil {
		t.Errorf("expected error with nil repo on Create")
	}
	if _, err := svc.Update(context.Background(), "1", UpdateInput{}); err == nil {
		t.Errorf("expected error with nil repo on Update")
	}
	if err := svc.Delete(context.Background(), "1"); err == nil {
		t.Errorf("expected error with nil repo on Delete")
	}
}

// -------------------------------------------------------------
// Audit Instrumentation Tests (Stage 3D)
// -------------------------------------------------------------

type mockActivityLogRepoForAutomation struct {
	recordedLogs []*domain.ActivityLog
	createErr    error
}

func (m *mockActivityLogRepoForAutomation) Create(ctx context.Context, log *domain.ActivityLog) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.recordedLogs = append(m.recordedLogs, log)
	return nil
}

func (m *mockActivityLogRepoForAutomation) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepoForAutomation) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func authContext(userID, role string) context.Context {
	p := &domain.Principal{UserID: userID, Role: role}
	return middleware.ContextWithPrincipal(context.Background(), p)
}

func TestService_Audit_Create_Success(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-aud-1"] = &domain.EmailTemplate{
		ID:           "tpl-aud-1",
		Name:         "Welcome Email Template",
		TemplateType: domain.TemplateTypeAutomation,
	}
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "admin")
	cfg := sampleConfig()
	input := CreateInput{
		Name:       "Welcome New Signups",
		TemplateID: "tpl-aud-1",
		Enabled:    boolPtr(true),
		Config:     cfg,
	}

	created, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAutomationCreated {
		t.Errorf("expected EventType %s, got %s", domain.EventAutomationCreated, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser || log.ActorUserID == nil || *log.ActorUserID != "usr-admin-1" {
		t.Errorf("unexpected actor: %+v", log)
	}
	if log.TargetType != domain.TargetTypeAutomationRule || log.TargetID != created.ID {
		t.Errorf("expected TargetType %s and TargetID %s, got type=%s id=%s", domain.TargetTypeAutomationRule, created.ID, log.TargetType, log.TargetID)
	}
	if log.TargetName == nil || *log.TargetName != "Welcome New Signups" {
		t.Errorf("expected target name 'Welcome New Signups', got %v", log.TargetName)
	}
	if log.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %s", log.Status)
	}
	if log.Summary != `Created automation rule "Welcome New Signups"` {
		t.Errorf("unexpected summary: %s", log.Summary)
	}

	var details map[string]any
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["id"] != created.ID || details["name"] != "Welcome New Signups" || details["templateId"] != "tpl-aud-1" || details["enabled"] != true {
		t.Errorf("unexpected snapshot details: %+v", details)
	}
}

func TestService_Audit_Create_RepoError_NoAudit(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	autoRepo.createErr = errors.New("db failure")
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-aud-1"] = &domain.EmailTemplate{
		ID:           "tpl-aud-1",
		TemplateType: domain.TemplateTypeAutomation,
	}
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "admin")
	_, err := svc.Create(ctx, CreateInput{
		Name:       "Rule",
		TemplateID: "tpl-aud-1",
		Config:     sampleConfig(),
	})
	if err == nil {
		t.Fatal("expected error on repo create failure, got nil")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on repo failure, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Create_MissingPrincipal_FailsFastNoMutation(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-aud-1"] = &domain.EmailTemplate{
		ID:           "tpl-aud-1",
		TemplateType: domain.TemplateTypeAutomation,
	}
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	// Context without principal
	_, err := svc.Create(context.Background(), CreateInput{
		Name:       "Unauth Rule",
		TemplateID: "tpl-aud-1",
		Config:     sampleConfig(),
	})
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal, got %v", err)
	}

	if len(autoRepo.rules) != 0 {
		t.Errorf("expected no rules persisted in repo on missing principal, got %d", len(autoRepo.rules))
	}
	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Create_AuditFailure_PropagatesError(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-aud-1"] = &domain.EmailTemplate{
		ID:           "tpl-aud-1",
		TemplateType: domain.TemplateTypeAutomation,
	}
	auditRepo := &mockActivityLogRepoForAutomation{
		createErr: errors.New("audit log write error"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "admin")
	_, err := svc.Create(ctx, CreateInput{
		Name:       "Rule",
		TemplateID: "tpl-aud-1",
		Config:     sampleConfig(),
	})
	if err == nil {
		t.Fatal("expected audit failure error, got nil")
	}
	if !strings.Contains(err.Error(), "audit log write error") {
		t.Errorf("expected audit error message, got %v", err)
	}
}

func TestService_Audit_Update_ConfigurationChange_Success(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-1"] = &domain.EmailTemplate{ID: "tpl-1", TemplateType: domain.TemplateTypeAutomation}
	tplRepo.templates["tpl-2"] = &domain.EmailTemplate{ID: "tpl-2", TemplateType: domain.TemplateTypeAutomation}

	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-upd-1",
		Name:       "Old Rule Name",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	newName := "New Rule Name"
	newTpl := "tpl-2"
	newCfg := sampleConfig()
	newCfg.Schedule.Type = domain.ScheduleTypeIntervalHours
	newCfg.Schedule.HourUTC = nil
	newCfg.Schedule.MinuteUTC = nil
	newCfg.Schedule.IntervalHours = intPtr(24)

	updated, err := svc.Update(ctx, "rule-upd-1", UpdateInput{
		Name:       &newName,
		TemplateID: &newTpl,
		Config:     &newCfg,
	})
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if updated.Name != "New Rule Name" || updated.TemplateID != "tpl-2" {
		t.Errorf("unexpected updated entity: %+v", updated)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAutomationUpdated {
		t.Errorf("expected EventType %s, got %s", domain.EventAutomationUpdated, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser || log.ActorUserID == nil || *log.ActorUserID != "usr-admin-1" {
		t.Errorf("unexpected actor: %+v", log)
	}
	if log.TargetType != domain.TargetTypeAutomationRule || log.TargetID != "rule-upd-1" {
		t.Errorf("expected TargetType %s and TargetID rule-upd-1, got type=%s id=%s", domain.TargetTypeAutomationRule, log.TargetType, log.TargetID)
	}
	if log.Summary != `Updated automation rule "New Rule Name"` {
		t.Errorf("unexpected summary: %s", log.Summary)
	}

	var details struct {
		Before map[string]any `json:"before"`
		After  map[string]any `json:"after"`
	}
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}

	if details.Before["name"] != "Old Rule Name" || details.Before["templateId"] != "tpl-1" {
		t.Errorf("unexpected before snapshot: %+v", details.Before)
	}
	if details.After["name"] != "New Rule Name" || details.After["templateId"] != "tpl-2" {
		t.Errorf("unexpected after snapshot: %+v", details.After)
	}
}

func TestService_Audit_Update_NoOp_NoAudit(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-1"] = &domain.EmailTemplate{ID: "tpl-1", TemplateType: domain.TemplateTypeAutomation}

	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	cfg := sampleConfig()
	existing := &domain.AutomationRule{
		ID:         "rule-noop-1",
		Name:       "Same Name",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     cfg,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	sameName := "Same Name"
	sameTpl := "tpl-1"
	sameEnabled := true
	sameCfg := sampleConfig()

	res, err := svc.Update(ctx, "rule-noop-1", UpdateInput{
		Name:       &sameName,
		TemplateID: &sameTpl,
		Enabled:    &sameEnabled,
		Config:     &sameCfg,
	})
	if err != nil {
		t.Fatalf("expected no-op update success, got %v", err)
	}
	if res.Name != "Same Name" {
		t.Errorf("unexpected result: %+v", res)
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on no-op update, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Update_MissingPrincipal_FailsFastNoMutation(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-upd-missing-auth",
		Name:       "Original Name",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	newName := "Hacked Name"
	_, err := svc.Update(context.Background(), "rule-upd-missing-auth", UpdateInput{
		Name: &newName,
	})
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal, got %v", err)
	}

	saved, _ := autoRepo.GetByID(context.Background(), "rule-upd-missing-auth")
	if saved.Name != "Original Name" {
		t.Errorf("expected rule in repo to remain unmodified, got %s", saved.Name)
	}
	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Update_AuditFailure_PropagatesError(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{
		createErr: errors.New("audit log error"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-upd-err",
		Name:       "Original Name",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	newName := "New Name"
	_, err := svc.Update(ctx, "rule-upd-err", UpdateInput{Name: &newName})
	if err == nil {
		t.Fatal("expected audit failure error, got nil")
	}
	if !strings.Contains(err.Error(), "audit log error") {
		t.Errorf("expected audit error message, got %v", err)
	}
}

func TestService_Audit_Enable_DisabledToEnabled(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-enable-1",
		Name:       "Inactive Rule",
		TemplateID: "tpl-1",
		Enabled:    false,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	updated, err := svc.Enable(ctx, "rule-enable-1")
	if err != nil {
		t.Fatalf("expected enable success, got %v", err)
	}
	if !updated.Enabled {
		t.Errorf("expected rule to be enabled, got false")
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAutomationEnabled {
		t.Errorf("expected EventType %s, got %s", domain.EventAutomationEnabled, log.EventType)
	}
	if log.Summary != `Enabled automation rule "Inactive Rule"` {
		t.Errorf("unexpected summary: %s", log.Summary)
	}

	var details map[string]any
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["previousEnabled"] != false || details["resultingEnabled"] != true || details["ruleId"] != "rule-enable-1" || details["ruleName"] != "Inactive Rule" {
		t.Errorf("unexpected enable details: %+v", details)
	}
}

func TestService_Audit_Enable_AlreadyEnabled_NoAudit(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-already-enabled",
		Name:       "Active Rule",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	updated, err := svc.Enable(ctx, "rule-already-enabled")
	if err != nil {
		t.Fatalf("expected enable success, got %v", err)
	}
	if !updated.Enabled {
		t.Errorf("expected rule to stay enabled, got false")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs for already-enabled rule, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Disable_EnabledToDisabled(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-disable-1",
		Name:       "Active Rule",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	updated, err := svc.Disable(ctx, "rule-disable-1")
	if err != nil {
		t.Fatalf("expected disable success, got %v", err)
	}
	if updated.Enabled {
		t.Errorf("expected rule to be disabled, got true")
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAutomationDisabled {
		t.Errorf("expected EventType %s, got %s", domain.EventAutomationDisabled, log.EventType)
	}
	if log.Summary != `Disabled automation rule "Active Rule"` {
		t.Errorf("unexpected summary: %s", log.Summary)
	}

	var details map[string]any
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["previousEnabled"] != true || details["resultingEnabled"] != false || details["ruleId"] != "rule-disable-1" || details["ruleName"] != "Active Rule" {
		t.Errorf("unexpected disable details: %+v", details)
	}
}

func TestService_Audit_Disable_AlreadyDisabled_NoAudit(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-already-disabled",
		Name:       "Disabled Rule",
		TemplateID: "tpl-1",
		Enabled:    false,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	updated, err := svc.Disable(ctx, "rule-already-disabled")
	if err != nil {
		t.Fatalf("expected disable success, got %v", err)
	}
	if updated.Enabled {
		t.Errorf("expected rule to stay disabled, got true")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs for already-disabled rule, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Delete_Success(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	cfg := sampleConfig()
	existing := &domain.AutomationRule{
		ID:         "rule-del-aud-1",
		Name:       "Rule To Delete",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     cfg,
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	err := svc.Delete(ctx, "rule-del-aud-1")
	if err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventAutomationDeleted {
		t.Errorf("expected EventType %s, got %s", domain.EventAutomationDeleted, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser || log.ActorUserID == nil || *log.ActorUserID != "usr-admin-1" {
		t.Errorf("unexpected actor: %+v", log)
	}
	if log.TargetType != domain.TargetTypeAutomationRule || log.TargetID != "rule-del-aud-1" {
		t.Errorf("expected TargetType %s and TargetID rule-del-aud-1, got type=%s id=%s", domain.TargetTypeAutomationRule, log.TargetType, log.TargetID)
	}
	if log.Summary != `Deleted automation rule "Rule To Delete"` {
		t.Errorf("unexpected summary: %s", log.Summary)
	}

	var details map[string]any
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["id"] != "rule-del-aud-1" || details["name"] != "Rule To Delete" || details["templateId"] != "tpl-1" || details["enabled"] != true {
		t.Errorf("unexpected deleted snapshot: %+v", details)
	}
}

func TestService_Audit_Delete_NotFound_NoAudit(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "admin")
	err := svc.Delete(ctx, "non-existent-rule-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on delete not found, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Delete_MissingPrincipal_FailsFastNoDeletion(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-del-noauth",
		Name:       "Rule",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	err := svc.Delete(context.Background(), "rule-del-noauth")
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal, got %v", err)
	}

	if _, ok := autoRepo.rules["rule-del-noauth"]; !ok {
		t.Errorf("rule was unexpectedly deleted from repository")
	}
	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs, got %d", len(auditRepo.recordedLogs))
	}
}

func TestService_Audit_Delete_AuditFailure_PropagatesError(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	auditRepo := &mockActivityLogRepoForAutomation{
		createErr: errors.New("audit failure on delete"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	existing := &domain.AutomationRule{
		ID:         "rule-del-err",
		Name:       "Rule",
		TemplateID: "tpl-1",
		Enabled:    true,
		Config:     sampleConfig(),
	}
	_ = autoRepo.Create(context.Background(), existing)

	ctx := authContext("usr-admin-1", "admin")
	err := svc.Delete(ctx, "rule-del-err")
	if err == nil {
		t.Fatal("expected audit failure error, got nil")
	}
	if !strings.Contains(err.Error(), "audit failure on delete") {
		t.Errorf("expected audit error message, got %v", err)
	}
}

func TestService_Audit_Privacy_NoSecretsInPayloads(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	tplRepo.templates["tpl-sec-1"] = &domain.EmailTemplate{
		ID:           "tpl-sec-1",
		TemplateType: domain.TemplateTypeAutomation,
	}
	auditRepo := &mockActivityLogRepoForAutomation{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(autoRepo, tplRepo, auditSvc)

	ctx := authContext("usr-admin-1", "admin")

	// 1. Create
	rule, err := svc.Create(ctx, CreateInput{
		Name:       "Security Test Rule",
		TemplateID: "tpl-sec-1",
		Config:     sampleConfig(),
	})
	if err != nil {
		t.Fatalf("failed create: %v", err)
	}

	// 2. Update
	newName := "Security Test Rule Modified"
	_, err = svc.Update(ctx, rule.ID, UpdateInput{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("failed update: %v", err)
	}

	// 3. Disable
	_, err = svc.Disable(ctx, rule.ID)
	if err != nil {
		t.Fatalf("failed disable: %v", err)
	}

	// 4. Enable
	_, err = svc.Enable(ctx, rule.ID)
	if err != nil {
		t.Fatalf("failed enable: %v", err)
	}

	// 5. Delete
	err = svc.Delete(ctx, rule.ID)
	if err != nil {
		t.Fatalf("failed delete: %v", err)
	}

	forbiddenWords := []string{"password", "secret", "smtp", "token", "jwt", "database", "postgres", "credentials"}
	for i, log := range auditRepo.recordedLogs {
		detailsJSON := string(log.Details)
		for _, word := range forbiddenWords {
			if strings.Contains(strings.ToLower(detailsJSON), word) {
				t.Errorf("log [%d] (%s) contained forbidden secret or keyword %q: %s", i, log.EventType, word, detailsJSON)
			}
		}
	}
}
