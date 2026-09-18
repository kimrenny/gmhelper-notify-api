package domain

import (
	"context"
	"time"
)

var (
	ErrNotFound = errorString("not found")
	ErrConflict = errorString("conflict")
)

type errorString string

func (e errorString) Error() string {
	return string(e)
}

type EmailTemplateRepository interface {
	GetByID(ctx context.Context, id string) (*EmailTemplate, error)
	GetByKey(ctx context.Context, templateKey string) (*EmailTemplate, error)
	Create(ctx context.Context, template *EmailTemplate) error
	Update(ctx context.Context, template *EmailTemplate) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*EmailTemplate, error)
}

type NotificationCampaignRepository interface {
	GetByID(ctx context.Context, id string) (*NotificationCampaign, error)
	Create(ctx context.Context, campaign *NotificationCampaign) error
	Update(ctx context.Context, campaign *NotificationCampaign) error
	Delete(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status CampaignStatus, startedAt, completedAt *time.Time) error
	ListByStatus(ctx context.Context, status CampaignStatus) ([]*NotificationCampaign, error)
	ListScheduled(ctx context.Context, after time.Time) ([]*NotificationCampaign, error)
	ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*NotificationCampaign, error)
	Claim(ctx context.Context, id string) (*NotificationCampaign, error)
	List(ctx context.Context) ([]*NotificationCampaign, error)
}

type CampaignDeliveryStats struct {
	TotalCount   int
	PendingCount int
	SendingCount int
	SentCount    int
	FailedCount  int
}

type CampaignRecipientRepository interface {
	GetByID(ctx context.Context, id string) (*CampaignRecipient, error)
	Create(ctx context.Context, recipient *CampaignRecipient) error
	ListByCampaignID(ctx context.Context, campaignID string) ([]*CampaignRecipient, error)
	UpdateStatus(ctx context.Context, id string, status DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error
	ClaimPending(ctx context.Context, limit int) ([]*CampaignRecipient, error)
	RecoverStaleSending(ctx context.Context, olderThan time.Duration) (int64, error)
	GetDeliveryStatsByCampaign(ctx context.Context, campaignID string) (*CampaignDeliveryStats, error)
}

type DirectNotificationRepository interface {
	GetByID(ctx context.Context, id string) (*DirectNotification, error)
	Create(ctx context.Context, notification *DirectNotification) error
	CreateWithInitialAttempt(ctx context.Context, notification *DirectNotification, attempt *DeliveryAttempt) error
	ListPending(ctx context.Context) ([]*DirectNotification, error)
	ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*DirectNotification, error)
	RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error)
	UpdateStatus(ctx context.Context, id string, status DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error
}

type DeliveryAttemptRepository interface {
	GetByID(ctx context.Context, id string) (*DeliveryAttempt, error)
	Create(ctx context.Context, attempt *DeliveryAttempt) error
	Update(ctx context.Context, attempt *DeliveryAttempt) error
	ListByTarget(ctx context.Context, targetType DeliveryTargetType, targetID string) ([]*DeliveryAttempt, error)
}

type AutomationRuleRepository interface {
	GetByID(ctx context.Context, id string) (*AutomationRule, error)
	Create(ctx context.Context, rule *AutomationRule) error
	Update(ctx context.Context, rule *AutomationRule) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*AutomationRule, error)
	ListEnabled(ctx context.Context) ([]*AutomationRule, error)
}

type AppSettingRepository interface {
	GetByKey(ctx context.Context, key string) (*AppSetting, error)
	Save(ctx context.Context, setting *AppSetting) error
	ListByCategory(ctx context.Context, category string) ([]*AppSetting, error)
	ListAll(ctx context.Context) ([]*AppSetting, error)
}

type DashboardRepository interface {
	GetDashboardStats(ctx context.Context, recentLimit int) (*DashboardStats, error)
}
