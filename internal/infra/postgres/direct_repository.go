package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/lib/pq"
)

type DirectNotificationRepository struct {
	db *sql.DB
}

func NewDirectNotificationRepository(db *sql.DB) *DirectNotificationRepository {
	return &DirectNotificationRepository{db: db}
}

func (r *DirectNotificationRepository) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	notification := &domain.DirectNotification{}
	var rawPayload []byte
	row := r.db.QueryRowContext(ctx, `
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE id = $1`, id)
	if err := row.Scan(&notification.ID, &notification.TemplateID, &notification.ExternalUserID, &notification.RecipientEmail, &notification.RecipientName, &notification.NotificationType, &notification.DeliveryStatus, &notification.AttemptsCount, &notification.LastAttemptAt, &notification.SentAt, &notification.ErrorMessage, &rawPayload, &notification.CreatedAt, &notification.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if len(rawPayload) > 0 {
		notification.Payload = json.RawMessage(rawPayload)
	}
	return notification, nil
}

func (r *DirectNotificationRepository) Create(ctx context.Context, notification *domain.DirectNotification) error {
	if err := notification.Validate(ctx); err != nil {
		return err
	}

	var payloadArg any
	if len(notification.Payload) > 0 {
		payloadArg = notification.Payload
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName,
		notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt,
		notification.SentAt, notification.ErrorMessage, payloadArg, notification.CreatedAt, notification.UpdatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23503" {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *DirectNotificationRepository) CreateWithInitialAttempt(ctx context.Context, notification *domain.DirectNotification, attempt *domain.DeliveryAttempt) error {
	if err := notification.Validate(ctx); err != nil {
		return err
	}
	if err := attempt.Validate(ctx); err != nil {
		return err
	}

	var payloadArg any
	if len(notification.Payload) > 0 {
		payloadArg = notification.Payload
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName,
		notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt,
		notification.SentAt, notification.ErrorMessage, payloadArg, notification.CreatedAt, notification.UpdatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23503" {
			return domain.ErrNotFound
		}
		return err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO delivery_attempts (id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber,
		attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DirectNotificationRepository) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE delivery_status = $1
ORDER BY created_at ASC`, domain.DeliveryStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := []*domain.DirectNotification{}
	for rows.Next() {
		notification := &domain.DirectNotification{}
		var rawPayload []byte
		if err := rows.Scan(&notification.ID, &notification.TemplateID, &notification.ExternalUserID, &notification.RecipientEmail, &notification.RecipientName, &notification.NotificationType, &notification.DeliveryStatus, &notification.AttemptsCount, &notification.LastAttemptAt, &notification.SentAt, &notification.ErrorMessage, &rawPayload, &notification.CreatedAt, &notification.UpdatedAt); err != nil {
			return nil, err
		}
		if len(rawPayload) > 0 {
			notification.Payload = json.RawMessage(rawPayload)
		}
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (r *DirectNotificationRepository) ClaimPending(ctx context.Context, limit int) ([]*domain.DirectNotification, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
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
          d.sent_at, d.error_message, d.payload, d.created_at, d.updated_at`

	rows, err := r.db.QueryContext(ctx, query, domain.DeliveryStatusPending, limit, domain.DeliveryStatusSending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := []*domain.DirectNotification{}
	for rows.Next() {
		notification := &domain.DirectNotification{}
		var rawPayload []byte
		if err := rows.Scan(
			&notification.ID, &notification.TemplateID, &notification.ExternalUserID,
			&notification.RecipientEmail, &notification.RecipientName, &notification.NotificationType,
			&notification.DeliveryStatus, &notification.AttemptsCount, &notification.LastAttemptAt,
			&notification.SentAt, &notification.ErrorMessage, &rawPayload,
			&notification.CreatedAt, &notification.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(rawPayload) > 0 {
			notification.Payload = json.RawMessage(rawPayload)
		}
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (r *DirectNotificationRepository) RecoverStaleSending(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, fmt.Errorf("olderThan duration must be positive: %v", olderThan)
	}

	query := `
WITH stale_notifications AS (
	SELECT id
	FROM direct_notifications
	WHERE delivery_status = $1
	  AND last_attempt_at IS NOT NULL
	  AND last_attempt_at < now() - ($2 * interval '1 microsecond')
	FOR UPDATE SKIP LOCKED
),
recovered_attempts AS (
	UPDATE delivery_attempts a
	SET status = $3,
	    error_message = 'delivery attempt timed out and was recovered'
	FROM stale_notifications s
	WHERE a.target_type = $4
	  AND a.target_id = s.id
	  AND (a.status = $1 OR a.status = $5)
),
updated_notifications AS (
	UPDATE direct_notifications d
	SET delivery_status = $5,
	    error_message = 'delivery claim timed out and was recovered',
	    updated_at = now()
	FROM stale_notifications s
	WHERE d.id = s.id
	RETURNING d.id
)
SELECT count(*) FROM updated_notifications`

	var count int64
	err := r.db.QueryRowContext(
		ctx,
		query,
		domain.DeliveryStatusSending,            // $1: sending
		olderThan.Microseconds(),                // $2: microseconds
		domain.DeliveryStatusFailed,             // $3: failed for interrupted attempts
		domain.DeliveryTargetDirectNotification, // $4: target_type
		domain.DeliveryStatusPending,            // $5: pending
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *DirectNotificationRepository) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	if !status.IsValid() {
		return domain.ErrInvalidEntity
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE direct_notifications
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`, status, attempts, lastAttemptAt, sentAt, errorMessage, id)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
