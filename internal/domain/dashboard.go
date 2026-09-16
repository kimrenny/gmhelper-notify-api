package domain

import "time"

// DashboardCampaignStats contains campaign counts grouped by status.
type DashboardCampaignStats struct {
	Total           int `json:"total"`
	Draft           int `json:"draft"`
	Scheduled       int `json:"scheduled"`
	Running         int `json:"running"`
	Sending         int `json:"sending"`
	Completed       int `json:"completed"`
	PartiallyFailed int `json:"partiallyFailed"`
	Failed          int `json:"failed"`
	Cancelled       int `json:"cancelled"`
}

// DashboardTemplateStats contains template counts grouped by status.
type DashboardTemplateStats struct {
	Total    int `json:"total"`
	Draft    int `json:"draft"`
	Active   int `json:"active"`
	Archived int `json:"archived"`
}

// DashboardDeliveryStats contains aggregated message delivery totals across campaign recipients and direct notifications.
type DashboardDeliveryStats struct {
	TotalMessages int     `json:"totalMessages"`
	TotalSent     int     `json:"totalSent"`
	TotalFailed   int     `json:"totalFailed"`
	TotalPending  int     `json:"totalPending"`
	TotalSending  int     `json:"totalSending"`
	SuccessRate   float64 `json:"successRate"`
}

// RecentCampaignItem represents a recent campaign summary entry for the dashboard overview.
type RecentCampaignItem struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	TemplateID   string         `json:"templateId"`
	TemplateName string         `json:"templateName,omitempty"`
	CampaignType string         `json:"campaignType"`
	Status       CampaignStatus `json:"status"`
	ScheduledAt  *time.Time     `json:"scheduledAt,omitempty"`
	StartedAt    *time.Time     `json:"startedAt,omitempty"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
}

// DashboardStats aggregates all metrics into a complete dashboard statistics domain object.
type DashboardStats struct {
	Campaigns       DashboardCampaignStats `json:"campaigns"`
	Templates       DashboardTemplateStats `json:"templates"`
	Deliveries      DashboardDeliveryStats `json:"deliveries"`
	RecentCampaigns []*RecentCampaignItem  `json:"recentCampaigns"`
}
