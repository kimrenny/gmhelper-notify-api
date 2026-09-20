package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestAutomationExecutionRepository_RecordExecution(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewAutomationExecutionRepository(db)
	now := time.Now().UTC()
	exec := &domain.AutomationExecution{
		ID:             "exec-1",
		RuleID:         "rule-100",
		EventID:        "evt-200",
		RecipientEmail: "user@example.com",
		Status:         "success",
		ExecutedAt:     now,
		CreatedAt:      now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO automation_executions (id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`)).
		WithArgs(exec.ID, exec.RuleID, exec.EventID, exec.RecipientEmail, exec.ExternalUserID, exec.NotificationID, exec.Status, exec.ExecutedAt, exec.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.RecordExecution(context.Background(), exec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationExecutionRepository_HasExecution(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewAutomationExecutionRepository(db)

	// 1. Found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT 1 FROM automation_executions
WHERE rule_id = $1 AND event_id = $2
LIMIT 1`)).
		WithArgs("rule-1", "evt-1").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	has, err := repo.HasExecution(context.Background(), "rule-1", "evt-1")
	if err != nil || !has {
		t.Fatalf("expected has=true, got has=%v, err=%v", has, err)
	}

	// 2. Not found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT 1 FROM automation_executions
WHERE rule_id = $1 AND event_id = $2
LIMIT 1`)).
		WithArgs("rule-1", "evt-2").
		WillReturnError(sql.ErrNoRows)

	has, err = repo.HasExecution(context.Background(), "rule-1", "evt-2")
	if err != nil || has {
		t.Fatalf("expected has=false, got has=%v, err=%v", has, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationExecutionRepository_GetLastSuccessfulExecution(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewAutomationExecutionRepository(db)
	now := time.Now().UTC()
	uid := "u-123"

	// 1. Found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at
FROM automation_executions
WHERE rule_id = $1 AND status = 'success' AND (
	recipient_email = $2 OR ($3 != '' AND external_user_id = $3)
)
ORDER BY executed_at DESC
LIMIT 1`)).
		WithArgs("rule-1", "user@example.com", "u-123").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rule_id", "event_id", "recipient_email", "external_user_id", "notification_id", "status", "executed_at", "created_at",
		}).AddRow("exec-1", "rule-1", "evt-1", "user@example.com", &uid, nil, "success", now, now))

	res, err := repo.GetLastSuccessfulExecution(context.Background(), "rule-1", "user@example.com", &uid)
	if err != nil || res == nil {
		t.Fatalf("expected result, got %v, err=%v", res, err)
	}
	if res.ID != "exec-1" {
		t.Errorf("expected exec-1, got %s", res.ID)
	}

	// 2. Not found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at
FROM automation_executions
WHERE rule_id = $1 AND status = 'success' AND (
	recipient_email = $2 OR ($3 != '' AND external_user_id = $3)
)
ORDER BY executed_at DESC
LIMIT 1`)).
		WithArgs("rule-1", "other@example.com", "").
		WillReturnError(sql.ErrNoRows)

	res, err = repo.GetLastSuccessfulExecution(context.Background(), "rule-1", "other@example.com", nil)
	if err != nil || res != nil {
		t.Fatalf("expected nil without error, got %v, err=%v", res, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAutomationExecutionRepository_ExecuteRuleAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewAutomationExecutionRepository(db)
	now := time.Now().UTC()
	cooldown := 7

	notif := &domain.DirectNotification{
		ID:               "notif-1",
		TemplateID:       "tpl-1",
		RecipientEmail:   "alex@example.com",
		NotificationType: domain.NotificationTypeAutomation,
		DeliveryStatus:   domain.DeliveryStatusPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	exec := &domain.AutomationExecution{
		ID:             "exec-1",
		RuleID:         "rule-1",
		EventID:        "evt-1",
		RecipientEmail: "alex@example.com",
		NotificationID: &notif.ID,
		Status:         "success",
		ExecutedAt:     now,
		CreatedAt:      now,
	}

	// 1. Success case
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`)).
		WithArgs("rule-1", "alex@example.com").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT status FROM automation_executions
WHERE rule_id = $1 AND event_id = $2
LIMIT 1`)).
		WithArgs("rule-1", "evt-1").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT executed_at FROM automation_executions
WHERE rule_id = $1 AND status = 'success' AND (
	recipient_email = $2 OR ($3 != '' AND external_user_id = $3)
)
ORDER BY executed_at DESC
LIMIT 1`)).
		WithArgs("rule-1", "alex@example.com", "").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(notif.ID, notif.TemplateID, notif.ExternalUserID, notif.RecipientEmail, notif.RecipientName, notif.NotificationType, notif.DeliveryStatus, notif.AttemptsCount, notif.LastAttemptAt, notif.SentAt, notif.ErrorMessage, nil, notif.CreatedAt, notif.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO automation_executions (id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`)).
		WithArgs(exec.ID, exec.RuleID, exec.EventID, exec.RecipientEmail, exec.ExternalUserID, exec.NotificationID, exec.Status, exec.ExecutedAt, exec.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	status, err := repo.ExecuteRuleAtomic(context.Background(), "rule-1", "evt-1", "alex@example.com", nil, &cooldown, now, notif, exec)
	if err != nil || status != "success" {
		t.Fatalf("expected status=success, got status=%s, err=%v", status, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
