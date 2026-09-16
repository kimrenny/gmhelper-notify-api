package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
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

func TestService_List_Success(t *testing.T) {
	autoRepo := newMockAutomationRepo()
	tplRepo := newMockTemplateRepo()
	svc := NewService(autoRepo, tplRepo)

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
	svc := NewService(autoRepo, tplRepo)

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
		ID:   "t-valid",
		Name: "Valid Template",
	}
	svc := NewService(autoRepo, tplRepo)

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
	tplRepo.templates["t-original"] = &domain.EmailTemplate{ID: "t-original"}
	tplRepo.templates["t-new"] = &domain.EmailTemplate{ID: "t-new"}

	svc := NewService(autoRepo, tplRepo)

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
	svc := NewService(autoRepo, tplRepo)

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
	autoRepo.deleteErr = errors.New("delete error")
	if err := svc.Delete(context.Background(), "rule-del"); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected delete error, got %v", err)
	}
}

func TestService_NilRepo(t *testing.T) {
	svc := NewService(nil, nil)

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
