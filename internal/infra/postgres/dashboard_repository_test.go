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

func TestDashboardRepository_GetDashboardStats_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewDashboardRepository(db)
	now := time.Now().UTC()

	// 1. Campaign counts
	campaignRows := sqlmock.NewRows([]string{"status", "count"}).
		AddRow("draft", 2).
		AddRow("scheduled", 3).
		AddRow("running", 1).
		AddRow("sending", 1).
		AddRow("completed", 5).
		AddRow("partially_failed", 1).
		AddRow("failed", 2).
		AddRow("cancelled", 1)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT status, COUNT(*)
FROM notification_campaigns
GROUP BY status`)).WillReturnRows(campaignRows)

	// 2. Template counts
	templateRows := sqlmock.NewRows([]string{"status", "count"}).
		AddRow("draft", 4).
		AddRow("active", 10).
		AddRow("archived", 2)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT status, COUNT(*)
FROM email_templates
GROUP BY status`)).WillReturnRows(templateRows)

	// 3. Campaign recipient delivery counts
	recipientRows := sqlmock.NewRows([]string{"delivery_status", "count"}).
		AddRow("sent", 80).
		AddRow("failed", 10).
		AddRow("pending", 5).
		AddRow("sending", 5)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT delivery_status, COUNT(*)
FROM campaign_recipients
GROUP BY delivery_status`)).WillReturnRows(recipientRows)

	// 4. Direct notification delivery counts
	directRows := sqlmock.NewRows([]string{"delivery_status", "count"}).
		AddRow("sent", 20).
		AddRow("failed", 5).
		AddRow("pending", 2).
		AddRow("sending", 1)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT delivery_status, COUNT(*)
FROM direct_notifications
GROUP BY delivery_status`)).WillReturnRows(directRows)

	// 5. Recent campaigns
	recentRows := sqlmock.NewRows([]string{
		"id", "name", "template_id", "template_name", "campaign_type", "status", "scheduled_at", "started_at", "completed_at", "created_at",
	}).AddRow("c-1", "Welcome Blast", "t-1", "Welcome Email", "broadcast", "completed", &now, &now, &now, now).
		AddRow("c-2", "Digest 2026", "t-2", "Monthly Digest", "recurring", "running", &now, &now, nil, now.Add(-1*time.Hour))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT c.id, c.name, c.template_id, COALESCE(t.name, ''), c.campaign_type, c.status, c.scheduled_at, c.started_at, c.completed_at, c.created_at
FROM notification_campaigns c
LEFT JOIN email_templates t ON c.template_id = t.id
ORDER BY c.created_at DESC
LIMIT $1`)).WithArgs(5).WillReturnRows(recentRows)

	stats, err := repo.GetDashboardStats(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Campaign Stats
	if stats.Campaigns.Total != 16 {
		t.Errorf("expected total campaigns 16, got %d", stats.Campaigns.Total)
	}
	if stats.Campaigns.Draft != 2 {
		t.Errorf("expected draft 2, got %d", stats.Campaigns.Draft)
	}
	if stats.Campaigns.Scheduled != 3 {
		t.Errorf("expected scheduled 3, got %d", stats.Campaigns.Scheduled)
	}
	if stats.Campaigns.Running != 1 {
		t.Errorf("expected running 1, got %d", stats.Campaigns.Running)
	}
	if stats.Campaigns.Completed != 5 {
		t.Errorf("expected completed 5, got %d", stats.Campaigns.Completed)
	}
	if stats.Campaigns.Failed != 2 {
		t.Errorf("expected failed 2, got %d", stats.Campaigns.Failed)
	}

	// Verify Template Stats
	if stats.Templates.Total != 16 {
		t.Errorf("expected total templates 16, got %d", stats.Templates.Total)
	}
	if stats.Templates.Active != 10 {
		t.Errorf("expected active templates 10, got %d", stats.Templates.Active)
	}
	if stats.Templates.Draft != 4 {
		t.Errorf("expected draft templates 4, got %d", stats.Templates.Draft)
	}

	// Verify Deliveries: (80 + 20) = 100 Sent, (10 + 5) = 15 Failed, (5 + 2) = 7 Pending, (5 + 1) = 6 Sending -> Total 128
	if stats.Deliveries.TotalMessages != 128 {
		t.Errorf("expected total messages 128, got %d", stats.Deliveries.TotalMessages)
	}
	if stats.Deliveries.TotalSent != 100 {
		t.Errorf("expected total sent 100, got %d", stats.Deliveries.TotalSent)
	}
	if stats.Deliveries.TotalFailed != 15 {
		t.Errorf("expected total failed 15, got %d", stats.Deliveries.TotalFailed)
	}
	if stats.Deliveries.TotalPending != 7 {
		t.Errorf("expected total pending 7, got %d", stats.Deliveries.TotalPending)
	}
	if stats.Deliveries.TotalSending != 6 {
		t.Errorf("expected total sending 6, got %d", stats.Deliveries.TotalSending)
	}
	// Success rate: 100 / (100 + 15) * 100 = 86.96%
	if stats.Deliveries.SuccessRate != 86.96 {
		t.Errorf("expected success rate 86.96, got %.2f", stats.Deliveries.SuccessRate)
	}

	// Verify Recent Campaigns
	if len(stats.RecentCampaigns) != 2 {
		t.Fatalf("expected 2 recent campaigns, got %d", len(stats.RecentCampaigns))
	}
	if stats.RecentCampaigns[0].Name != "Welcome Blast" || stats.RecentCampaigns[0].TemplateName != "Welcome Email" {
		t.Errorf("unexpected recent campaign 0: %+v", stats.RecentCampaigns[0])
	}
	if stats.RecentCampaigns[0].Status != domain.CampaignStatusCompleted {
		t.Errorf("expected status completed, got %s", stats.RecentCampaigns[0].Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

func TestDashboardRepository_GetDashboardStats_EmptyDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewDashboardRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM email_templates GROUP BY status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM campaign_recipients GROUP BY delivery_status`)).
		WillReturnRows(sqlmock.NewRows([]string{"delivery_status", "count"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM direct_notifications GROUP BY delivery_status`)).
		WillReturnRows(sqlmock.NewRows([]string{"delivery_status", "count"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT c.id, c.name, c.template_id, COALESCE(t.name, ''), c.campaign_type, c.status, c.scheduled_at, c.started_at, c.completed_at, c.created_at`)).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "template_id", "template_name", "campaign_type", "status", "scheduled_at", "started_at", "completed_at", "created_at"}))

	// Passing 0 should default to limit 5
	stats, err := repo.GetDashboardStats(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Campaigns.Total != 0 {
		t.Errorf("expected total campaigns 0, got %d", stats.Campaigns.Total)
	}
	if stats.Templates.Total != 0 {
		t.Errorf("expected total templates 0, got %d", stats.Templates.Total)
	}
	if stats.Deliveries.TotalMessages != 0 || stats.Deliveries.SuccessRate != 0.0 {
		t.Errorf("expected total messages 0 and rate 0.0, got %d and %.2f", stats.Deliveries.TotalMessages, stats.Deliveries.SuccessRate)
	}
	if stats.RecentCampaigns == nil || len(stats.RecentCampaigns) != 0 {
		t.Errorf("expected empty non-nil recent campaigns slice, got: %v", stats.RecentCampaigns)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", err)
	}
}

func TestDashboardRepository_GetDashboardStats_QueryErrors(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
	}{
		{
			name: "campaigns query error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
					WillReturnError(errors.New("db query error"))
			},
		},
		{
			name: "templates query error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM email_templates GROUP BY status`)).
					WillReturnError(errors.New("templates db error"))
			},
		},
		{
			name: "recipients query error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM email_templates GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM campaign_recipients GROUP BY delivery_status`)).
					WillReturnError(errors.New("recipients query error"))
			},
		},
		{
			name: "direct notifications query error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM email_templates GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM campaign_recipients GROUP BY delivery_status`)).
					WillReturnRows(sqlmock.NewRows([]string{"delivery_status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM direct_notifications GROUP BY delivery_status`)).
					WillReturnError(errors.New("direct query error"))
			},
		},
		{
			name: "recent campaigns query error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM notification_campaigns GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, COUNT(*) FROM email_templates GROUP BY status`)).
					WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM campaign_recipients GROUP BY delivery_status`)).
					WillReturnRows(sqlmock.NewRows([]string{"delivery_status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT delivery_status, COUNT(*) FROM direct_notifications GROUP BY delivery_status`)).
					WillReturnRows(sqlmock.NewRows([]string{"delivery_status", "count"}))
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT c.id, c.name, c.template_id, COALESCE(t.name, ''), c.campaign_type, c.status, c.scheduled_at, c.started_at, c.completed_at, c.created_at`)).
					WithArgs(5).
					WillReturnError(errors.New("recent campaigns query error"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock database: %v", err)
			}
			defer db.Close()

			tc.setupMock(mock)

			repo := NewDashboardRepository(db)
			_, err = repo.GetDashboardStats(context.Background(), 5)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
