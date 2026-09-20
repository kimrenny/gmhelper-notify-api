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

func TestAutomationExecutionRepository_ListByRuleID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewAutomationExecutionRepository(db)
	now := time.Now().UTC()
	uid := "u-1"
	notifID := "notif-1"

	// 1. Success with records and deterministic sorting
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COUNT(*) FROM automation_executions
WHERE rule_id = $1`)).
		WithArgs("rule-100").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at
FROM automation_executions
WHERE rule_id = $1
ORDER BY executed_at DESC, id DESC
LIMIT $2 OFFSET $3`)).
		WithArgs("rule-100", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rule_id", "event_id", "recipient_email", "external_user_id", "notification_id", "status", "executed_at", "created_at",
		}).
			AddRow("exec-2", "rule-100", "evt-2", "user2@example.com", &uid, &notifID, "success", now, now).
			AddRow("exec-1", "rule-100", "evt-1", "user1@example.com", nil, nil, "skipped_cooldown", now.Add(-time.Hour), now.Add(-time.Hour)))

	items, total, err := repo.ListByRuleID(context.Background(), "rule-100", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected total=2 and 2 items, got total=%d, len=%d", total, len(items))
	}
	if items[0].ID != "exec-2" || items[0].Status != "success" || items[0].NotificationID == nil || *items[0].NotificationID != "notif-1" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].ID != "exec-1" || items[1].Status != "skipped_cooldown" {
		t.Errorf("unexpected second item: %+v", items[1])
	}

	// 2. Empty results (total = 0)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COUNT(*) FROM automation_executions
WHERE rule_id = $1`)).
		WithArgs("rule-empty").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	emptyItems, emptyTotal, err := repo.ListByRuleID(context.Background(), "rule-empty", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error for empty rule: %v", err)
	}
	if emptyTotal != 0 || len(emptyItems) != 0 {
		t.Errorf("expected 0 items, got total=%d, len=%d", emptyTotal, len(emptyItems))
	}

	// 3. Empty rule ID returns empty without query
	blankItems, blankTotal, err := repo.ListByRuleID(context.Background(), "   ", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blankTotal != 0 || len(blankItems) != 0 {
		t.Errorf("expected 0 items for blank rule ID")
	}

	// 4. Equal timestamp ID tie-breaker verification
	sameTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COUNT(*) FROM automation_executions
WHERE rule_id = $1`)).
		WithArgs("rule-tie").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at
FROM automation_executions
WHERE rule_id = $1
ORDER BY executed_at DESC, id DESC
LIMIT $2 OFFSET $3`)).
		WithArgs("rule-tie", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rule_id", "event_id", "recipient_email", "external_user_id", "notification_id", "status", "executed_at", "created_at",
		}).
			AddRow("exec-b", "rule-tie", "evt-b", "b@example.com", nil, nil, "success", sameTime, sameTime).
			AddRow("exec-a", "rule-tie", "evt-a", "a@example.com", nil, nil, "success", sameTime, sameTime))

	tieItems, _, err := repo.ListByRuleID(context.Background(), "rule-tie", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tieItems) != 2 || tieItems[0].ID != "exec-b" || tieItems[1].ID != "exec-a" {
		t.Errorf("expected deterministic ordering by id DESC for equal timestamps: %+v", tieItems)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
