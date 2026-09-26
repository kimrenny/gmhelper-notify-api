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
		AudienceFilter: &domain.CampaignAudienceFilter{
			Role: "admin",
		},
		ScheduledAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO notification_campaigns (id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)).
		WithArgs(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.AudienceFilter, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), campaign); err != nil {
		t.Fatalf("failed to create campaign: %v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, `{"role":"admin"}`, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
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
	if fetched.AudienceFilter == nil || fetched.AudienceFilter.Role != "admin" {
		t.Fatalf("expected audience filter role 'admin', got %+v", fetched.AudienceFilter)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_CreateAndGet_Unscheduled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()
	campaign := &domain.NotificationCampaign{
		ID:             "a1b2c3d4-e5f6-7890-abcd-ef1234567891",
		Name:           "Draft Unscheduled Campaign",
		TemplateID:     "f7ba18f7-4c2a-4b77-8565-1e1e5d64047f",
		CampaignType:   "broadcast",
		Status:         domain.CampaignStatusDraft,
		AudienceFilter: nil,
		ScheduledAt:    nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO notification_campaigns (id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)).
		WithArgs(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, nil, nil, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), campaign); err != nil {
		t.Fatalf("failed to create unscheduled campaign: %v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow(campaign.ID, campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, nil, nil, campaign.StartedAt, campaign.CompletedAt, campaign.CreatedAt, campaign.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE id = $1`)).
		WithArgs(campaign.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), campaign.ID)
	if err != nil {
		t.Fatalf("failed to fetch campaign: %v", err)
	}
	if fetched.ScheduledAt != nil {
		t.Fatalf("expected nil scheduledAt, got %v", fetched.ScheduledAt)
	}
	if fetched.AudienceFilter != nil {
		t.Fatalf("expected nil AudienceFilter, got %v", fetched.AudienceFilter)
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
		AudienceFilter: &domain.CampaignAudienceFilter{
			Language: "tr",
		},
		ScheduledAt: &now,
		UpdatedAt:   now,
	}

	// 1. Successful Update
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE notification_campaigns
SET name = $1, template_id = $2, campaign_type = $3, status = $4, audience_filter = $5, scheduled_at = $6, updated_at = $7
WHERE id = $8`)).
		WithArgs(campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.AudienceFilter, campaign.ScheduledAt, campaign.UpdatedAt, campaign.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Update(context.Background(), campaign); err != nil {
		t.Fatalf("failed to update campaign: %v", err)
	}

	// 2. Not Found Update (0 rows affected)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE notification_campaigns
SET name = $1, template_id = $2, campaign_type = $3, status = $4, audience_filter = $5, scheduled_at = $6, updated_at = $7
WHERE id = $8`)).
		WithArgs(campaign.Name, campaign.TemplateID, campaign.CampaignType, campaign.Status, campaign.AudienceFilter, campaign.ScheduledAt, campaign.UpdatedAt, campaign.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	errNotFound := repo.Update(context.Background(), campaign)
	if errNotFound != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", errNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	campaignID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	now := time.Now().UTC()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE notification_campaigns
SET status = $1, started_at = COALESCE($2, started_at), completed_at = COALESCE($3, completed_at), updated_at = now()
WHERE id = $4`)).
		WithArgs(domain.CampaignStatusRunning, &now, nil, campaignID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.UpdateStatus(context.Background(), campaignID, domain.CampaignStatusRunning, &now, nil); err != nil {
		t.Fatalf("failed to update status to running: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_ListByStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow("camp-1", "Scheduled Camp", "tpl-1", "broadcast", domain.CampaignStatusScheduled, nil, &now, nil, nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE status = $1`)).
		WithArgs(domain.CampaignStatusScheduled).
		WillReturnRows(rows)

	res, err := repo.ListByStatus(context.Background(), domain.CampaignStatusScheduled)
	if err != nil {
		t.Fatalf("failed to list by status: %v", err)
	}
	if len(res) != 1 || res[0].Status != domain.CampaignStatusScheduled {
		t.Fatalf("unexpected list result: %+v", res)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_ListScheduled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow("camp-1", "Scheduled Camp", "tpl-1", "broadcast", domain.CampaignStatusScheduled, nil, &now, nil, nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE scheduled_at >= $1
ORDER BY scheduled_at ASC`)).
		WithArgs(now).
		WillReturnRows(rows)

	res, err := repo.ListScheduled(context.Background(), now)
	if err != nil {
		t.Fatalf("failed to list scheduled: %v", err)
	}
	if len(res) != 1 || res[0].ScheduledAt == nil {
		t.Fatalf("unexpected list scheduled result: %+v", res)
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

func TestNotificationCampaignRepository_ListDue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()
	schedTime1 := now.Add(-10 * time.Minute)
	schedTime2 := now.Add(-5 * time.Minute)

	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow("camp-1", "Due Camp 1", "tpl-1", "broadcast", domain.CampaignStatusScheduled, nil, &schedTime1, nil, nil, now, now).
		AddRow("camp-2", "Due Camp 2", "tpl-1", "broadcast", domain.CampaignStatusScheduled, nil, &schedTime2, nil, nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at
FROM notification_campaigns
WHERE status = $1
  AND scheduled_at IS NOT NULL
  AND scheduled_at <= $2
ORDER BY scheduled_at ASC, id ASC
LIMIT $3`)).
		WithArgs(domain.CampaignStatusScheduled, now, 10).
		WillReturnRows(rows)

	res, err := repo.ListDue(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("failed to list due campaigns: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 due campaigns, got %d", len(res))
	}
	if res[0].ID != "camp-1" || res[1].ID != "camp-2" {
		t.Fatalf("unexpected order: %+v", res)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestNotificationCampaignRepository_Claim(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewNotificationCampaignRepository(db)
	now := time.Now().UTC()
	campaignID := "camp-due-1"

	// 1. Successful Claim
	rows := sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}).
		AddRow(campaignID, "Due Camp 1", "tpl-1", "broadcast", domain.CampaignStatusRunning, nil, &now, nil, nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
UPDATE notification_campaigns
SET status = $1, updated_at = now()
WHERE id = $2
  AND status = $3
RETURNING id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at`)).
		WithArgs(domain.CampaignStatusRunning, campaignID, domain.CampaignStatusScheduled).
		WillReturnRows(rows)

	claimed, err := repo.Claim(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("failed to claim campaign: %v", err)
	}
	if claimed.Status != domain.CampaignStatusRunning {
		t.Fatalf("expected status running, got %s", claimed.Status)
	}

	// 2. Unsuccessful Claim (already running or cancelled, 0 rows returned)
	mock.ExpectQuery(regexp.QuoteMeta(`
UPDATE notification_campaigns
SET status = $1, updated_at = now()
WHERE id = $2
  AND status = $3
RETURNING id, name, template_id, campaign_type, status, audience_filter, scheduled_at, started_at, completed_at, created_at, updated_at`)).
		WithArgs(domain.CampaignStatusRunning, campaignID, domain.CampaignStatusScheduled).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "template_id", "campaign_type", "status", "audience_filter", "scheduled_at", "started_at", "completed_at", "created_at", "updated_at"}))

	_, errConflict := repo.Claim(context.Background(), campaignID)
	if !errors.Is(errConflict, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for already claimed campaign, got %v", errConflict)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
