package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestDirectNotificationRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDirectNotificationRepository(db)
	now := time.Now().UTC()
	notification := &domain.DirectNotification{
		ID:               "direct-uuid-1",
		TemplateID:       "template-uuid-1",
		ExternalUserID:   "user-101",
		RecipientEmail:   "user@example.com",
		RecipientName:    "Alice",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    0,
		Payload:          json.RawMessage(`{"name":"Alice"}`),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// 1. Create - Validation error
	invalid := &domain.DirectNotification{}
	if err := repo.Create(context.Background(), invalid); !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity on invalid notification, got %v", err)
	}

	// 2. Create - Success
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName, notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt, notification.SentAt, notification.ErrorMessage, notification.Payload, notification.CreatedAt, notification.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), notification); err != nil {
		t.Fatalf("failed to create notification: %v", err)
	}

	// 3. GetByID - Found
	rows := sqlmock.NewRows([]string{
		"id", "template_id", "external_user_id", "recipient_email", "recipient_name", "notification_type",
		"delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "payload", "created_at", "updated_at",
	}).AddRow(
		notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName,
		notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt,
		notification.SentAt, notification.ErrorMessage, []byte(`{"name":"Alice"}`), notification.CreatedAt, notification.UpdatedAt,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE id = $1`)).
		WithArgs(notification.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), notification.ID)
	if err != nil {
		t.Fatalf("failed to get notification: %v", err)
	}
	if fetched.RecipientEmail != notification.RecipientEmail {
		t.Fatalf("expected recipient email %s, got %s", notification.RecipientEmail, fetched.RecipientEmail)
	}

	// 4. GetByID - Missing (ErrNotFound)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE id = $1`)).
		WithArgs("missing-id").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByID(context.Background(), "missing-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing notification, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestDirectNotificationRepository_UpdateStatusAndListPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDirectNotificationRepository(db)
	now := time.Now().UTC()

	// 1. UpdateStatus - Success
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE direct_notifications
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`)).
		WithArgs(domain.DeliveryStatusSent, 1, &now, &now, "", "notif-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.UpdateStatus(context.Background(), "notif-1", domain.DeliveryStatusSent, 1, &now, &now, ""); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// 2. UpdateStatus - Not Found (rows affected 0)
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE direct_notifications
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`)).
		WithArgs(domain.DeliveryStatusFailed, 2, &now, nil, "smtp error", "notif-missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.UpdateStatus(context.Background(), "notif-missing", domain.DeliveryStatusFailed, 2, &now, nil, "smtp error")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. UpdateStatus - Invalid Status
	if err := repo.UpdateStatus(context.Background(), "notif-1", "invalid_status", 1, nil, nil, ""); !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity, got %v", err)
	}

	// 4. ListPending - Success
	rows := sqlmock.NewRows([]string{
		"id", "template_id", "external_user_id", "recipient_email", "recipient_name", "notification_type",
		"delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "payload", "created_at", "updated_at",
	}).AddRow(
		"notif-1", "tmpl-1", "user-1", "a@example.com", "A", domain.NotificationTypeDirect,
		domain.DeliveryStatusPending, 0, nil, nil, "", []byte(`{}`), now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE delivery_status = $1
ORDER BY created_at ASC`)).
		WithArgs(domain.DeliveryStatusPending).
		WillReturnRows(rows)

	list, err := repo.ListPending(context.Background())
	if err != nil {
		t.Fatalf("failed to list pending: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestDirectNotificationRepository_CreateWithInitialAttempt_Atomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDirectNotificationRepository(db)
	now := time.Now().UTC()

	notification := &domain.DirectNotification{
		ID:               "direct-tx-1",
		TemplateID:       "template-1",
		RecipientEmail:   "user@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		Payload:          json.RawMessage(`{"key":"value"}`),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-tx-1",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notification.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   now,
		CreatedAt:     now,
	}

	// 1. Successful transaction: Commit
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName, notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt, notification.SentAt, notification.ErrorMessage, notification.Payload, notification.CreatedAt, notification.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO delivery_attempts (id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)).
		WithArgs(attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber, attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	if err := repo.CreateWithInitialAttempt(context.Background(), notification, attempt); err != nil {
		t.Fatalf("expected atomic creation success, got %v", err)
	}

	// 2. Failed attempt insert -> Rollback
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName, notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt, notification.SentAt, notification.ErrorMessage, notification.Payload, notification.CreatedAt, notification.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO delivery_attempts (id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)).
		WithArgs(attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber, attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt).
		WillReturnError(errors.New("db disk full"))

	mock.ExpectRollback()

	err = repo.CreateWithInitialAttempt(context.Background(), notification, attempt)
	if err == nil {
		t.Fatal("expected error on attempt insert failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestDirectNotificationRepository_ClaimPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDirectNotificationRepository(db)
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "template_id", "external_user_id", "recipient_email", "recipient_name", "notification_type",
		"delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "payload", "created_at", "updated_at",
	}).AddRow(
		"claimed-id-1", "template-1", "user-1", "recipient@example.com", "Recipient",
		domain.NotificationTypeDirect, domain.DeliveryStatusSending, 1, &now,
		nil, "", []byte(`{"key":"value"}`), now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
WITH claimed AS (
	SELECT id
	FROM direct_notifications
	WHERE delivery_status = $1
	ORDER BY created_at ASC
	FOR UPDATE SKIP LOCKED
	LIMIT $2
)
UPDATE direct_notifications d
SET delivery_status = $3,
    attempts_count = d.attempts_count + 1,
    last_attempt_at = now(),
    updated_at = now()
FROM claimed
WHERE d.id = claimed.id
RETURNING d.id, d.template_id, d.external_user_id, d.recipient_email, d.recipient_name,
          d.notification_type, d.delivery_status, d.attempts_count, d.last_attempt_at,
          d.sent_at, d.error_message, d.payload, d.created_at, d.updated_at`)).
		WithArgs(domain.DeliveryStatusPending, 5, domain.DeliveryStatusSending).
		WillReturnRows(rows)

	claimed, err := repo.ClaimPending(context.Background(), 5)
	if err != nil {
		t.Fatalf("expected ClaimPending success, got: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed notification, got %d", len(claimed))
	}
	if claimed[0].ID != "claimed-id-1" {
		t.Errorf("expected ID claimed-id-1, got %s", claimed[0].ID)
	}
	if claimed[0].DeliveryStatus != domain.DeliveryStatusSending {
		t.Errorf("expected status sending, got %s", claimed[0].DeliveryStatus)
	}
	if claimed[0].AttemptsCount != 1 {
		t.Errorf("expected attempts_count 1, got %d", claimed[0].AttemptsCount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
