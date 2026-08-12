package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type DirectNotificationRepository struct {
	db *sql.DB
}

func NewDirectNotificationRepository(db *sql.DB) *DirectNotificationRepository {
	return &DirectNotificationRepository{db: db}
}

func (r *DirectNotificationRepository) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	notification := &domain.DirectNotification{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE id = $1`, id)
	if err := row.Scan(&notification.ID, &notification.TemplateID, &notification.ExternalUserID, &notification.RecipientEmail, &notification.RecipientName, &notification.NotificationType, &notification.DeliveryStatus, &notification.AttemptsCount, &notification.LastAttemptAt, &notification.SentAt, &notification.ErrorMessage, &notification.Payload, &notification.CreatedAt, &notification.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return notification, nil
}

func (r *DirectNotificationRepository) Create(ctx context.Context, notification *domain.DirectNotification) error {
	if err := notification.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`, notification.ID, notification.TemplateID, notification.ExternalUserID, notification.RecipientEmail, notification.RecipientName, notification.NotificationType, notification.DeliveryStatus, notification.AttemptsCount, notification.LastAttemptAt, notification.SentAt, notification.ErrorMessage, notification.Payload, notification.CreatedAt, notification.UpdatedAt)
	return err
}

func (r *DirectNotificationRepository) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at
FROM direct_notifications
WHERE delivery_status = $1`, domain.DeliveryStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := []*domain.DirectNotification{}
	for rows.Next() {
		notification := &domain.DirectNotification{}
		if err := rows.Scan(&notification.ID, &notification.TemplateID, &notification.ExternalUserID, &notification.RecipientEmail, &notification.RecipientName, &notification.NotificationType, &notification.DeliveryStatus, &notification.AttemptsCount, &notification.LastAttemptAt, &notification.SentAt, &notification.ErrorMessage, &notification.Payload, &notification.CreatedAt, &notification.UpdatedAt); err != nil {
			return nil, err
		}
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (r *DirectNotificationRepository) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	if !status.IsValid() {
		return domain.ErrInvalidEntity
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE direct_notifications
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`, status, attempts, lastAttemptAt, sentAt, errorMessage, id)
	return err
}
