package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type CampaignRecipientRepository struct {
	db *sql.DB
}

func NewCampaignRecipientRepository(db *sql.DB) *CampaignRecipientRepository {
	return &CampaignRecipientRepository{db: db}
}

func (r *CampaignRecipientRepository) GetByID(ctx context.Context, id string) (*domain.CampaignRecipient, error) {
	recipient := &domain.CampaignRecipient{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at
FROM campaign_recipients
WHERE id = $1`, id)
	if err := row.Scan(&recipient.ID, &recipient.CampaignID, &recipient.ExternalUserID, &recipient.RecipientEmail, &recipient.RecipientName, &recipient.DeliveryStatus, &recipient.AttemptsCount, &recipient.LastAttemptAt, &recipient.SentAt, &recipient.ErrorMessage, &recipient.CreatedAt, &recipient.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return recipient, nil
}

func (r *CampaignRecipientRepository) Create(ctx context.Context, recipient *domain.CampaignRecipient) error {
	if err := recipient.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO campaign_recipients (id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (campaign_id, external_user_id) DO NOTHING`, recipient.ID, recipient.CampaignID, recipient.ExternalUserID, recipient.RecipientEmail, recipient.RecipientName, recipient.DeliveryStatus, recipient.AttemptsCount, recipient.LastAttemptAt, recipient.SentAt, recipient.ErrorMessage, recipient.CreatedAt, recipient.UpdatedAt)
	return err
}

func (r *CampaignRecipientRepository) ListByCampaignID(ctx context.Context, campaignID string) ([]*domain.CampaignRecipient, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at
FROM campaign_recipients
WHERE campaign_id = $1
ORDER BY created_at ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipients := []*domain.CampaignRecipient{}
	for rows.Next() {
		recipient := &domain.CampaignRecipient{}
		if err := rows.Scan(&recipient.ID, &recipient.CampaignID, &recipient.ExternalUserID, &recipient.RecipientEmail, &recipient.RecipientName, &recipient.DeliveryStatus, &recipient.AttemptsCount, &recipient.LastAttemptAt, &recipient.SentAt, &recipient.ErrorMessage, &recipient.CreatedAt, &recipient.UpdatedAt); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}
	return recipients, rows.Err()
}

func (r *CampaignRecipientRepository) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	if !status.IsValid() {
		return domain.ErrInvalidEntity
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE campaign_recipients
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`, status, attempts, lastAttemptAt, sentAt, errorMessage, id)
	return err
}

func (r *CampaignRecipientRepository) ClaimPending(ctx context.Context, limit int) ([]*domain.CampaignRecipient, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
WITH claimed AS (
	SELECT cr.id
	FROM campaign_recipients cr
	JOIN notification_campaigns nc ON nc.id = cr.campaign_id
	WHERE nc.status = 'running'
	  AND cr.delivery_status = $1
	ORDER BY cr.created_at ASC
	FOR UPDATE OF cr SKIP LOCKED
	LIMIT $2
)
UPDATE campaign_recipients cr
SET delivery_status = $3,
    attempts_count = cr.attempts_count + 1,
    last_attempt_at = now(),
    updated_at = now()
FROM claimed
WHERE cr.id = claimed.id
RETURNING cr.id, cr.campaign_id, cr.external_user_id, cr.recipient_email, cr.recipient_name, cr.delivery_status, cr.attempts_count, cr.last_attempt_at, cr.sent_at, cr.error_message, cr.created_at, cr.updated_at`

	rows, err := r.db.QueryContext(ctx, query, domain.DeliveryStatusPending, limit, domain.DeliveryStatusSending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipients := []*domain.CampaignRecipient{}
	for rows.Next() {
		recipient := &domain.CampaignRecipient{}
		if err := rows.Scan(&recipient.ID, &recipient.CampaignID, &recipient.ExternalUserID, &recipient.RecipientEmail, &recipient.RecipientName, &recipient.DeliveryStatus, &recipient.AttemptsCount, &recipient.LastAttemptAt, &recipient.SentAt, &recipient.ErrorMessage, &recipient.CreatedAt, &recipient.UpdatedAt); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}

	return recipients, rows.Err()
}

func (r *CampaignRecipientRepository) RecoverStaleSending(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	res, err := r.db.ExecContext(ctx, `
UPDATE campaign_recipients
SET delivery_status = $1, updated_at = now()
WHERE delivery_status = $2
  AND last_attempt_at < $3`, domain.DeliveryStatusPending, domain.DeliveryStatusSending, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *CampaignRecipientRepository) GetDeliveryStatsByCampaign(ctx context.Context, campaignID string) (*domain.CampaignDeliveryStats, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT
	COUNT(*),
	COUNT(*) FILTER (WHERE delivery_status = 'pending'),
	COUNT(*) FILTER (WHERE delivery_status = 'sending'),
	COUNT(*) FILTER (WHERE delivery_status = 'sent'),
	COUNT(*) FILTER (WHERE delivery_status = 'failed')
FROM campaign_recipients
WHERE campaign_id = $1`, campaignID)

	stats := &domain.CampaignDeliveryStats{}
	if err := row.Scan(&stats.TotalCount, &stats.PendingCount, &stats.SendingCount, &stats.SentCount, &stats.FailedCount); err != nil {
		return nil, err
	}
	return stats, nil
}
