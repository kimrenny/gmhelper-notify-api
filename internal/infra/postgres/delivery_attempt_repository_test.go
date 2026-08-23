package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestDeliveryAttemptRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDeliveryAttemptRepository(db)
	now := time.Now().UTC()
	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-101",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      "direct-101",
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		ErrorMessage:  "",
		AttemptedAt:   now,
		CreatedAt:     now,
	}

	// 1. Create - Validation error
	invalid := &domain.DeliveryAttempt{}
	if err := repo.Create(context.Background(), invalid); !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity on invalid attempt, got %v", err)
	}

	// 2. Create - Success
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO delivery_attempts (id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)).
		WithArgs(attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber, attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), attempt); err != nil {
		t.Fatalf("failed to create attempt: %v", err)
	}

	// 3. GetByID - Success
	rows := sqlmock.NewRows([]string{"id", "target_type", "target_id", "status", "attempt_number", "error_message", "attempted_at", "created_at"}).
		AddRow(attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber, attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at
FROM delivery_attempts
WHERE id = $1`)).
		WithArgs(attempt.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), attempt.ID)
	if err != nil {
		t.Fatalf("failed to get attempt: %v", err)
	}
	if fetched.TargetID != attempt.TargetID {
		t.Fatalf("expected TargetID %s, got %s", attempt.TargetID, fetched.TargetID)
	}

	// 4. GetByID - Not Found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at
FROM delivery_attempts
WHERE id = $1`)).
		WithArgs("missing-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "target_type", "target_id", "status", "attempt_number", "error_message", "attempted_at", "created_at"}))

	_, err = repo.GetByID(context.Background(), "missing-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing attempt, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestDeliveryAttemptRepository_UpdateAndList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDeliveryAttemptRepository(db)
	now := time.Now().UTC()
	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-202",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      "direct-202",
		Status:        domain.DeliveryStatusSent,
		AttemptNumber: 1,
		ErrorMessage:  "",
		AttemptedAt:   now,
		CreatedAt:     now,
	}

	// 1. Update - Success
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE delivery_attempts
SET status = $1, error_message = $2, attempted_at = $3
WHERE id = $4`)).
		WithArgs(attempt.Status, attempt.ErrorMessage, attempt.AttemptedAt, attempt.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Update(context.Background(), attempt); err != nil {
		t.Fatalf("failed to update attempt: %v", err)
	}

	// 2. Update - Not Found (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE delivery_attempts
SET status = $1, error_message = $2, attempted_at = $3
WHERE id = $4`)).
		WithArgs(attempt.Status, attempt.ErrorMessage, attempt.AttemptedAt, attempt.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Update(context.Background(), attempt)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on 0 rows affected, got %v", err)
	}

	// 3. ListByTarget - Success
	rows := sqlmock.NewRows([]string{"id", "target_type", "target_id", "status", "attempt_number", "error_message", "attempted_at", "created_at"}).
		AddRow("attempt-1", domain.DeliveryTargetDirectNotification, "direct-202", domain.DeliveryStatusFailed, 1, "timeout", now, now).
		AddRow("attempt-2", domain.DeliveryTargetDirectNotification, "direct-202", domain.DeliveryStatusSent, 2, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at
FROM delivery_attempts
WHERE target_type = $1 AND target_id = $2
ORDER BY attempt_number ASC`)).
		WithArgs(domain.DeliveryTargetDirectNotification, "direct-202").
		WillReturnRows(rows)

	list, err := repo.ListByTarget(context.Background(), domain.DeliveryTargetDirectNotification, "direct-202")
	if err != nil {
		t.Fatalf("failed to list attempts by target: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(list))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
