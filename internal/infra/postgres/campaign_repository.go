package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type NotificationCampaignRepository struct {
	db *sql.DB
}

func NewNotificationCampaignRepository(db *sql.DB) *NotificationCampaignRepository {
	return &NotificationCampaignRepository{db: db}
}

func (r *NotificationCampaignRepository) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	campaign := &domain.NotificationCampaign{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE id = $1`, id)
	if err := row.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return campaign, nil
}

func (r *NotificationCampaignRepository) Create(ctx context.Context, campaign *domain.NotificationCampaign) error {
	if err := campaign.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO notification_campaigns (id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.AudienceFilter, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt)
	return err
}

func (r *NotificationCampaignRepository) Update(ctx context.Context, campaign *domain.NotificationCampaign) error {
	if err := campaign.Validate(ctx); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE notification_campaigns
SET name = $1, template_id = $2, campaign_type = $3, status = $4, audience_filter = $5, scheduled_at = $6, updated_at = $7
WHERE id = $8`, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.AudienceFilter, campaign.ScheduledAt, campaign.UpdatedAt, campaign.ID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *NotificationCampaignRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM notification_campaigns WHERE id = $1`, id)
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

func (r *NotificationCampaignRepository) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	if !status.IsValid() {
		return domain.ErrInvalidEntity
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE notification_campaigns
SET status = $1, started_at = COALESCE($2, started_at), completed_at = COALESCE($3, completed_at), updated_at = now()
WHERE id = $4`, status, startedAt, completedAt, id)
	return err
}

func (r *NotificationCampaignRepository) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE status = $1`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []*domain.NotificationCampaign{}
	for rows.Next() {
		campaign := &domain.NotificationCampaign{}
		if err := rows.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, campaign)
	}
	return campaigns, rows.Err()
}

func (r *NotificationCampaignRepository) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE scheduled_at >= $1
ORDER BY scheduled_at ASC`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []*domain.NotificationCampaign{}
	for rows.Next() {
		campaign := &domain.NotificationCampaign{}
		if err := rows.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, campaign)
	}
	return campaigns, rows.Err()
}

func (r *NotificationCampaignRepository) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []*domain.NotificationCampaign{}
	for rows.Next() {
		campaign := &domain.NotificationCampaign{}
		if err := rows.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, campaign)
	}
	return campaigns, rows.Err()
}

func (r *NotificationCampaignRepository) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE status = $1
  AND scheduled_at IS NOT NULL
  AND scheduled_at <= $2
ORDER BY scheduled_at ASC, id ASC
LIMIT $3`, domain.CampaignStatusScheduled, dueBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []*domain.NotificationCampaign{}
	for rows.Next() {
		campaign := &domain.NotificationCampaign{}
		if err := rows.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, campaign)
	}
	return campaigns, rows.Err()
}

func (r *NotificationCampaignRepository) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	campaign := &domain.NotificationCampaign{}
	row := r.db.QueryRowContext(ctx, `
UPDATE notification_campaigns
SET status = $1, updated_at = now()
WHERE id = $2
  AND status = $3
RETURNING id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at`,
		domain.CampaignStatusRunning, id, domain.CampaignStatusScheduled)

	if err := row.Scan(&campaign.ID, &campaign.Name, &campaign.TemplateID, &campaign.CampaignType, &campaign.Status, &campaign.AudienceFilter, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return campaign, nil
}
