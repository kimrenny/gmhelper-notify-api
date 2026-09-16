package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func intPtr(i int) *int {
	return &i
}

func sampleValidRule(id, name, tplID string, enabled bool) *domain.AutomationRule {
	now := time.Now().UTC()
	return &domain.AutomationRule{
		ID:         id,
		Name:       name,
		TemplateID: tplID,
		Enabled:    enabled,
		Config: domain.AutomationRuleConfig{
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
				Type: domain.ActionTypeSendEmail,
			},
		},
		LastEvaluatedAt:  nil,
		NextEvaluationAt: nil,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func TestAutomationRuleRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAutomationRuleRepository(db)
	rule := sampleValidRule("rule-1111-2222-3333-4444", "User Inactive Re-engagement", "tpl-1111-2222-3333-4444", true)

	// 1. Validation error on Create
	invalidRule := &domain.AutomationRule{ID: ""}
	if err := repo.Create(context.Background(), invalidRule); err == nil {
		t.Errorf("expected validation error on invalid rule, got nil")
	}

	// 2. Successful Create
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO automation_rules (id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`)).
		WithArgs(rule.ID, rule.Name, rule.TemplateID, rule.Enabled, rule.Config, rule.LastEvaluatedAt, rule.NextEvaluationAt, rule.CreatedAt, rule.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), rule); err != nil {
		t.Fatalf("failed to create automation rule: %v", err)
	}

	cfgBytes, _ := json.Marshal(rule.Config)

	// 3. Successful GetByID
	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}).
		AddRow(rule.ID, rule.Name, rule.TemplateID, rule.Enabled, cfgBytes, rule.LastEvaluatedAt, rule.NextEvaluationAt, rule.CreatedAt, rule.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
WHERE id = $1`)).
		WithArgs(rule.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("failed to get automation rule: %v", err)
	}
	if fetched.ID != rule.ID || fetched.Name != rule.Name || fetched.TemplateID != rule.TemplateID || !fetched.Enabled {
		t.Fatalf("unexpected fetched rule: %+v", fetched)
	}

	// 4. GetByID Not Found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
WHERE id = $1`)).
		WithArgs("non-existent-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}))

	notFound, err := repo.GetByID(context.Background(), "non-existent-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v, rule=%+v", err, notFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationRuleRepository_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAutomationRuleRepository(db)
	rule := sampleValidRule("rule-1", "Updated Rule Name", "tpl-2", false)

	// 1. Validation error on Update
	invalidRule := &domain.AutomationRule{ID: ""}
	if err := repo.Update(context.Background(), invalidRule); err == nil {
		t.Errorf("expected validation error on invalid rule, got nil")
	}

	// 2. Successful Update
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE automation_rules
SET name = $1, template_id = $2, enabled = $3, config = $4, last_evaluated_at = $5, next_evaluation_at = $6, updated_at = now()
WHERE id = $7`)).
		WithArgs(rule.Name, rule.TemplateID, rule.Enabled, rule.Config, rule.LastEvaluatedAt, rule.NextEvaluationAt, rule.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Update(context.Background(), rule); err != nil {
		t.Fatalf("failed to update automation rule: %v", err)
	}

	// 3. Update Not Found (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE automation_rules
SET name = $1, template_id = $2, enabled = $3, config = $4, last_evaluated_at = $5, next_evaluation_at = $6, updated_at = now()
WHERE id = $7`)).
		WithArgs(rule.Name, rule.TemplateID, rule.Enabled, rule.Config, rule.LastEvaluatedAt, rule.NextEvaluationAt, rule.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.Update(context.Background(), rule); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on 0 rows affected, got %v", err)
	}

	// 4. Update Database Error
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE automation_rules
SET name = $1, template_id = $2, enabled = $3, config = $4, last_evaluated_at = $5, next_evaluation_at = $6, updated_at = now()
WHERE id = $7`)).
		WithArgs(rule.Name, rule.TemplateID, rule.Enabled, rule.Config, rule.LastEvaluatedAt, rule.NextEvaluationAt, rule.ID).
		WillReturnError(errors.New("db connection failure"))

	if err := repo.Update(context.Background(), rule); err == nil {
		t.Fatalf("expected error on DB failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationRuleRepository_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAutomationRuleRepository(db)

	// 1. Successful Delete
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM automation_rules WHERE id = $1`)).
		WithArgs("rule-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Delete(context.Background(), "rule-123"); err != nil {
		t.Fatalf("failed to delete rule: %v", err)
	}

	// 2. Delete Not Found (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM automation_rules WHERE id = $1`)).
		WithArgs("rule-404").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.Delete(context.Background(), "rule-404"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent rule, got %v", err)
	}

	// 3. Delete Database Error
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM automation_rules WHERE id = $1`)).
		WithArgs("rule-err").
		WillReturnError(errors.New("db foreign key constraint error"))

	if err := repo.Delete(context.Background(), "rule-err"); err == nil {
		t.Fatalf("expected error on DB failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationRuleRepository_List(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAutomationRuleRepository(db)
	now := time.Now().UTC()
	earlier := now.Add(-1 * time.Hour)

	r1 := sampleValidRule("rule-1", "Welcome Series", "tpl-1", true)
	r1.CreatedAt = earlier
	r2 := sampleValidRule("rule-2", "Re-engagement", "tpl-2", false)
	r2.CreatedAt = now

	cfgBytes1, _ := json.Marshal(r1.Config)
	cfgBytes2, _ := json.Marshal(r2.Config)

	// 1. List returns multiple rules (both enabled and disabled) with deterministic ordering
	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}).
		AddRow(r2.ID, r2.Name, r2.TemplateID, r2.Enabled, cfgBytes2, r2.LastEvaluatedAt, r2.NextEvaluationAt, now, now).
		AddRow(r1.ID, r1.Name, r1.TemplateID, r1.Enabled, cfgBytes1, r1.LastEvaluatedAt, r1.NextEvaluationAt, earlier, earlier)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
ORDER BY created_at DESC, id DESC`)).
		WillReturnRows(rows)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("failed to list automation rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].ID != "rule-2" || rules[0].Enabled != false {
		t.Errorf("expected first rule to be rule-2 (disabled), got %+v", rules[0])
	}
	if rules[1].ID != "rule-1" || rules[1].Enabled != true {
		t.Errorf("expected second rule to be rule-1 (enabled), got %+v", rules[1])
	}

	// 2. List returns initialized empty slice when no rows exist
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
ORDER BY created_at DESC, id DESC`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}))

	emptyRules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("failed to list empty rules: %v", err)
	}
	if emptyRules == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(emptyRules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(emptyRules))
	}

	// 3. List propagates database query error
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
ORDER BY created_at DESC, id DESC`)).
		WillReturnError(errors.New("query failure"))

	if _, err := repo.List(context.Background()); err == nil {
		t.Fatalf("expected error on DB failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationRuleRepository_ListEnabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAutomationRuleRepository(db)
	r1 := sampleValidRule("rule-enabled-1", "Welcome Series", "tpl-1", true)
	cfgBytes, _ := json.Marshal(r1.Config)

	// 1. ListEnabled returns only enabled rules
	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}).
		AddRow(r1.ID, r1.Name, r1.TemplateID, r1.Enabled, cfgBytes, r1.LastEvaluatedAt, r1.NextEvaluationAt, r1.CreatedAt, r1.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
WHERE enabled = true
ORDER BY created_at DESC, id DESC`)).
		WillReturnRows(rows)

	rules, err := repo.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("failed to list enabled rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "rule-enabled-1" || !rules[0].Enabled {
		t.Fatalf("unexpected enabled rules: %+v", rules)
	}

	// 2. ListEnabled returns initialized empty slice when no enabled rows exist
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
WHERE enabled = true
ORDER BY created_at DESC, id DESC`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "template_id", "enabled", "config", "last_evaluated_at", "next_evaluation_at", "created_at", "updated_at"}))

	emptyRules, err := repo.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("failed to list empty enabled rules: %v", err)
	}
	if emptyRules == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(emptyRules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(emptyRules))
	}

	// 3. ListEnabled propagates database query error
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, enabled, config, last_evaluated_at, next_evaluation_at, created_at, updated_at
FROM automation_rules
WHERE enabled = true
ORDER BY created_at DESC, id DESC`)).
		WillReturnError(errors.New("db error"))

	if _, err := repo.ListEnabled(context.Background()); err == nil {
		t.Fatalf("expected error on DB failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
