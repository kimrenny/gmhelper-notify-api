package postgres

import (
	"context"
	"database/sql"
	"math"

	"github.com/gmhelper/notify-api/internal/domain"
)

// DashboardRepository provides PostgreSQL aggregation queries for dashboard statistics.
type DashboardRepository struct {
	db *sql.DB
}

// NewDashboardRepository constructs a new DashboardRepository.
func NewDashboardRepository(db *sql.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// GetDashboardStats aggregates metrics across campaigns, templates, direct notifications, and recipient delivery data.
func (r *DashboardRepository) GetDashboardStats(ctx context.Context, recentLimit int) (*domain.DashboardStats, error) {
	if recentLimit <= 0 {
		recentLimit = 5
	}

	stats := &domain.DashboardStats{
		RecentCampaigns: make([]*domain.RecentCampaignItem, 0),
	}

	// 1. Campaign counts by status
	campaignRows, err := r.db.QueryContext(ctx, `
SELECT status, COUNT(*)
FROM notification_campaigns
GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer campaignRows.Close()

	for campaignRows.Next() {
		var status string
		var count int
		if err := campaignRows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.Campaigns.Total += count
		switch domain.CampaignStatus(status) {
		case domain.CampaignStatusDraft:
			stats.Campaigns.Draft = count
		case domain.CampaignStatusScheduled:
			stats.Campaigns.Scheduled = count
		case domain.CampaignStatusRunning:
			stats.Campaigns.Running = count
		case domain.CampaignStatusSending:
			stats.Campaigns.Sending = count
		case domain.CampaignStatusCompleted:
			stats.Campaigns.Completed = count
		case domain.CampaignStatusPartiallyFailed:
			stats.Campaigns.PartiallyFailed = count
		case domain.CampaignStatusFailed:
			stats.Campaigns.Failed = count
		case domain.CampaignStatusCancelled:
			stats.Campaigns.Cancelled = count
		}
	}
	if err := campaignRows.Err(); err != nil {
		return nil, err
	}

	// 2. Template counts by status
	templateRows, err := r.db.QueryContext(ctx, `
SELECT status, COUNT(*)
FROM email_templates
GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer templateRows.Close()

	for templateRows.Next() {
		var status string
		var count int
		if err := templateRows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.Templates.Total += count
		switch domain.TemplateStatus(status) {
		case domain.TemplateStatusDraft:
			stats.Templates.Draft = count
		case domain.TemplateStatusActive:
			stats.Templates.Active = count
		case domain.TemplateStatusArchived:
			stats.Templates.Archived = count
		}
	}
	if err := templateRows.Err(); err != nil {
		return nil, err
	}

	// 3. Delivery counts from campaign recipients
	recipientRows, err := r.db.QueryContext(ctx, `
SELECT delivery_status, COUNT(*)
FROM campaign_recipients
GROUP BY delivery_status`)
	if err != nil {
		return nil, err
	}
	defer recipientRows.Close()

	for recipientRows.Next() {
		var status string
		var count int
		if err := recipientRows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.Deliveries.TotalMessages += count
		switch domain.DeliveryStatus(status) {
		case domain.DeliveryStatusSent:
			stats.Deliveries.TotalSent += count
		case domain.DeliveryStatusFailed:
			stats.Deliveries.TotalFailed += count
		case domain.DeliveryStatusPending:
			stats.Deliveries.TotalPending += count
		case domain.DeliveryStatusSending:
			stats.Deliveries.TotalSending += count
		}
	}
	if err := recipientRows.Err(); err != nil {
		return nil, err
	}

	// 4. Delivery counts from direct notifications
	directRows, err := r.db.QueryContext(ctx, `
SELECT delivery_status, COUNT(*)
FROM direct_notifications
GROUP BY delivery_status`)
	if err != nil {
		return nil, err
	}
	defer directRows.Close()

	for directRows.Next() {
		var status string
		var count int
		if err := directRows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.Deliveries.TotalMessages += count
		switch domain.DeliveryStatus(status) {
		case domain.DeliveryStatusSent:
			stats.Deliveries.TotalSent += count
		case domain.DeliveryStatusFailed:
			stats.Deliveries.TotalFailed += count
		case domain.DeliveryStatusPending:
			stats.Deliveries.TotalPending += count
		case domain.DeliveryStatusSending:
			stats.Deliveries.TotalSending += count
		}
	}
	if err := directRows.Err(); err != nil {
		return nil, err
	}

	// Calculate success rate based on processed messages (Sent vs Failed)
	processed := stats.Deliveries.TotalSent + stats.Deliveries.TotalFailed
	if processed > 0 {
		rate := (float64(stats.Deliveries.TotalSent) / float64(processed)) * 100.0
		stats.Deliveries.SuccessRate = math.Round(rate*100) / 100
	}

	// 5. Recent campaigns (latest N ordered by created_at DESC)
	recentRows, err := r.db.QueryContext(ctx, `
SELECT c.id, c.name, c.template_id, COALESCE(t.name, ''), c.campaign_type, c.status, c.scheduled_at, c.started_at, c.completed_at, c.created_at
FROM notification_campaigns c
LEFT JOIN email_templates t ON c.template_id = t.id
ORDER BY c.created_at DESC
LIMIT $1`, recentLimit)
	if err != nil {
		return nil, err
	}
	defer recentRows.Close()

	for recentRows.Next() {
		item := &domain.RecentCampaignItem{}
		var status string
		if err := recentRows.Scan(
			&item.ID,
			&item.Name,
			&item.TemplateID,
			&item.TemplateName,
			&item.CampaignType,
			&status,
			&item.ScheduledAt,
			&item.StartedAt,
			&item.CompletedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.CampaignStatus(status)
		stats.RecentCampaigns = append(stats.RecentCampaigns, item)
	}
	if err := recentRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}
