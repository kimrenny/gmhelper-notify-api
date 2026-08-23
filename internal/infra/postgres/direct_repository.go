package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
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
