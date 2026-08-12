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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, recipient.ID, recipient.CampaignID, recipient.ExternalUserID, recipient.RecipientEmail, recipient.RecipientName, recipient.DeliveryStatus, recipient.AttemptsCount, recipient.LastAttemptAt, recipient.SentAt, recipient.ErrorMessage, recipient.CreatedAt, recipient.UpdatedAt)
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
