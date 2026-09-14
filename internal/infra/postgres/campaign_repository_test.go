package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestNotificationCampaignRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()
	campaign := &domain.NotificationCampaign{
		ID:           "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Name:         "Welcome Campaign",
		TemplateID:   "f7ba18f7-4c2a-4b77-8565-1e1e5d64047f",
		CampaignType: "broadcast",
		Status:       domain.CampaignStatusDraft,
		ScheduledAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO notification_campaigns (id, name, template_id, campaign_type, status, scheduled_at, started_at, completed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`)).
		WithArgs(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), campaign); err != nil {
		t.Fatalf("failed to create campaign: %v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, campaign_type, status, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE id = $1`)).
		WithArgs(campaign.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), campaign.ID)
	if err != nil {
		t.Fatalf("failed to fetch campaign: %v", err)
	}
	if fetched.Name != campaign.Name {
		t.Fatalf("expected campaign name %s, got %s", campaign.Name, fetched.Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()
	campaign := &domain.NotificationCampaign{
		ID:           "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Name:         "Updated Campaign Name",
		TemplateID:   "f7ba18f7-4c2a-4b77-8565-1e1e5d64047f",
		CampaignType: "broadcast",
		Status:       domain.CampaignStatusScheduled,
		ScheduledAt:  now,
		UpdatedAt:    now,
	}

	// 1. Successful Update
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE notification_campaigns
SET name = $1, template_id = $2, campaign_type = $3, status = $4, scheduled_at = $5, updated_at = $6
WHERE id = $7`)).
		WithArgs(campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.ScheduledAt, campaign.UpdatedAt, campaign.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Update(context.Background(), campaign); err != nil {
		t.Fatalf("failed to update campaign: %v", err)
	}

	// 2. Not Found Update (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE notification_campaigns
SET name = $1, template_id = $2, campaign_type = $3, status = $4, scheduled_at = $5, updated_at = $6
WHERE id = $7`)).
		WithArgs(campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.ScheduledAt, campaign.UpdatedAt, campaign.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	errNotFound := repo.Update(context.Background(), campaign)
	if errNotFound != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", errNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	campaignID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

	// 1. Successful Delete
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM notification_campaigns WHERE id = $1`)).
		WithArgs(campaignID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Delete(context.Background(), campaignID); err != nil {
		t.Fatalf("failed to delete campaign: %v", err)
	}

	// 2. Not Found Delete (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM notification_campaigns WHERE id = $1`)).
		WithArgs(campaignID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	errNotFound := repo.Delete(context.Background(), campaignID)
	if errNotFound != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", errNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
