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

func TestCampaignRecipientRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewCampaignRecipientRepository(db)
	now := time.Now().UTC()
	recipient := &domain.CampaignRecipient{
		ID:             "rec-1",
		CampaignID:     "cmp-1",
		ExternalUserID: "usr-1",
		RecipientEmail: "user@example.com",
		RecipientName:  "Test User",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// 1. Create - Validation error
	invalid := &domain.CampaignRecipient{}
	if err := repo.Create(context.Background(), invalid); !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity on invalid recipient, got %v", err)
	}

	// 2. Create - Success
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO campaign_recipients (id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (campaign_id, external_user_id) DO NOTHING`)).
		WithArgs(recipient.ID, recipient.CampaignID, recipient.ExternalUserID, recipient.RecipientEmail, recipient.RecipientName, recipient.DeliveryStatus, recipient.AttemptsCount, recipient.LastAttemptAt, recipient.SentAt, recipient.ErrorMessage, recipient.CreatedAt, recipient.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), recipient); err != nil {
		t.Fatalf("failed to create recipient: %v", err)
	}

	// 3. GetByID - Success
	rows := sqlmock.NewRows([]string{"id", "campaign_id", "external_user_id", "recipient_email", "recipient_name", "delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "created_at", "updated_at"}).
		AddRow(recipient.ID, recipient.CampaignID, recipient.ExternalUserID, recipient.RecipientEmail, recipient.RecipientName, recipient.DeliveryStatus, recipient.AttemptsCount, recipient.LastAttemptAt, recipient.SentAt, recipient.ErrorMessage, recipient.CreatedAt, recipient.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at
FROM campaign_recipients
WHERE id = $1`)).
		WithArgs(recipient.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), recipient.ID)
	if err != nil {
		t.Fatalf("failed to get recipient: %v", err)
	}
	if fetched.RecipientEmail != recipient.RecipientEmail {
		t.Fatalf("expected email %s, got %s", recipient.RecipientEmail, fetched.RecipientEmail)
	}

	// 4. GetByID - Not found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at
FROM campaign_recipients
WHERE id = $1`)).
		WithArgs("missing-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "campaign_id", "external_user_id", "recipient_email", "recipient_name", "delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "created_at", "updated_at"}))

	_, err = repo.GetByID(context.Background(), "missing-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing recipient, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestCampaignRecipientRepository_ListByCampaignID_AndUpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewCampaignRecipientRepository(db)
	now := time.Now().UTC()

	// 1. ListByCampaignID
	rows := sqlmock.NewRows([]string{"id", "campaign_id", "external_user_id", "recipient_email", "recipient_name", "delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "created_at", "updated_at"}).
		AddRow("rec-1", "cmp-1", "usr-1", "u1@example.com", "U1", domain.DeliveryStatusPending, 0, nil, nil, "", now, now).
		AddRow("rec-2", "cmp-1", "usr-2", "u2@example.com", "U2", domain.DeliveryStatusSent, 1, &now, &now, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, campaign_id, external_user_id, recipient_email, recipient_name, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, created_at, updated_at
FROM campaign_recipients
WHERE campaign_id = $1
ORDER BY created_at ASC`)).
		WithArgs("cmp-1").
		WillReturnRows(rows)

	list, err := repo.ListByCampaignID(context.Background(), "cmp-1")
	if err != nil {
		t.Fatalf("failed to list recipients: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 recipients, got %d", len(list))
	}

	// 2. UpdateStatus - invalid status
	err = repo.UpdateStatus(context.Background(), "rec-1", domain.DeliveryStatus("invalid"), 1, &now, &now, "")
	if !errors.Is(err, domain.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity on invalid status, got %v", err)
	}

	// 3. UpdateStatus - valid
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE campaign_recipients
SET delivery_status = $1, attempts_count = $2, last_attempt_at = $3, sent_at = $4, error_message = $5, updated_at = now()
WHERE id = $6`)).
		WithArgs(domain.DeliveryStatusSent, 1, &now, &now, "", "rec-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpdateStatus(context.Background(), "rec-1", domain.DeliveryStatusSent, 1, &now, &now, "")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestCampaignRecipientRepository_ClaimPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewCampaignRecipientRepository(db)
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{"id", "campaign_id", "external_user_id", "recipient_email", "recipient_name", "delivery_status", "attempts_count", "last_attempt_at", "sent_at", "error_message", "created_at", "updated_at"}).
		AddRow("rec-1", "cmp-1", "usr-1", "u1@example.com", "U1", domain.DeliveryStatusSending, 1, &now, nil, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`
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
RETURNING cr.id, cr.campaign_id, cr.external_user_id, cr.recipient_email, cr.recipient_name, cr.delivery_status, cr.attempts_count, cr.last_attempt_at, cr.sent_at, cr.error_message, cr.created_at, cr.updated_at`)).
		WithArgs(domain.DeliveryStatusPending, 10, domain.DeliveryStatusSending).
		WillReturnRows(rows)

	claimed, err := repo.ClaimPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("failed to claim pending recipients: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed recipient, got %d", len(claimed))
	}
	if claimed[0].DeliveryStatus != domain.DeliveryStatusSending {
		t.Fatalf("expected status 'sending', got %s", claimed[0].DeliveryStatus)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestCampaignRecipientRepository_RecoverStaleSending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewCampaignRecipientRepository(db)

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE campaign_recipients
SET delivery_status = $1, updated_at = now()
WHERE delivery_status = $2
  AND last_attempt_at < $3`)).
		WithArgs(domain.DeliveryStatusPending, domain.DeliveryStatusSending, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	recovered, err := repo.RecoverStaleSending(context.Background(), 5*time.Minute)
	if err != nil {
		t.Fatalf("failed to recover stale sending: %v", err)
	}
	if recovered != 3 {
		t.Fatalf("expected 3 recovered rows, got %d", recovered)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestCampaignRecipientRepository_GetDeliveryStatsByCampaign(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewCampaignRecipientRepository(db)

	rows := sqlmock.NewRows([]string{"total", "pending", "sending", "sent", "failed"}).
		AddRow(10, 2, 1, 5, 2)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT
	COUNT(*),
	COUNT(*) FILTER (WHERE delivery_status = 'pending'),
	COUNT(*) FILTER (WHERE delivery_status = 'sending'),
	COUNT(*) FILTER (WHERE delivery_status = 'sent'),
	COUNT(*) FILTER (WHERE delivery_status = 'failed')
FROM campaign_recipients
WHERE campaign_id = $1`)).
		WithArgs("cmp-1").
		WillReturnRows(rows)

	stats, err := repo.GetDeliveryStatsByCampaign(context.Background(), "cmp-1")
	if err != nil {
		t.Fatalf("failed to get delivery stats: %v", err)
	}
	if stats.TotalCount != 10 || stats.PendingCount != 2 || stats.SendingCount != 1 || stats.SentCount != 5 || stats.FailedCount != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
