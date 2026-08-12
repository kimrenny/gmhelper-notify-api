package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type TemplateStatus string

type CampaignStatus string

type DeliveryStatus string

type NotificationType string

type DeliveryTargetType string

const (
	TemplateStatusDraft    TemplateStatus = "draft"
	TemplateStatusActive   TemplateStatus = "active"
	TemplateStatusArchived TemplateStatus = "archived"
)

const (
	CampaignStatusDraft          CampaignStatus = "draft"
	CampaignStatusScheduled      CampaignStatus = "scheduled"
	CampaignStatusSending        CampaignStatus = "sending"
	CampaignStatusCompleted      CampaignStatus = "completed"
	CampaignStatusPartiallyFailed CampaignStatus = "partially_failed"
	CampaignStatusFailed         CampaignStatus = "failed"
	CampaignStatusCancelled      CampaignStatus = "cancelled"
)

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusSending   DeliveryStatus = "sending"
	DeliveryStatusSent      DeliveryStatus = "sent"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusCancelled DeliveryStatus = "cancelled"
)

const (
	NotificationTypeDirect        NotificationType = "direct"
	NotificationTypeUserAgreement NotificationType = "user_agreement"
)

const (
	DeliveryTargetCampaignRecipient DeliveryTargetType = "campaign_recipient"
	DeliveryTargetDirectNotification DeliveryTargetType = "direct_notification"
)

var (
	ErrInvalidEntity = errors.New("invalid entity")
)

func (s TemplateStatus) IsValid() bool {
	switch s {
	case TemplateStatusDraft, TemplateStatusActive, TemplateStatusArchived:
		return true
	default:
		return false
	}
}

func (s CampaignStatus) IsValid() bool {
	switch s {
	case CampaignStatusDraft, CampaignStatusScheduled, CampaignStatusSending, CampaignStatusCompleted, CampaignStatusPartiallyFailed, CampaignStatusFailed, CampaignStatusCancelled:
		return true
	default:
		return false
	}
}

func (s DeliveryStatus) IsValid() bool {
	switch s {
	case DeliveryStatusPending, DeliveryStatusSending, DeliveryStatusSent, DeliveryStatusFailed, DeliveryStatusCancelled:
		return true
	default:
		return false
	}
}

func (t NotificationType) IsValid() bool {
	switch t {
	case NotificationTypeDirect, NotificationTypeUserAgreement:
		return true
	default:
		return false
	}
}

func (t DeliveryTargetType) IsValid() bool {
	switch t {
	case DeliveryTargetCampaignRecipient, DeliveryTargetDirectNotification:
		return true
	default:
		return false
	}
}

func (t *EmailTemplate) Validate(ctx context.Context) error {
	if t.ID == "" || t.TemplateKey == "" || t.Name == "" || t.Subject == "" || t.HTMLBody == "" || t.Locale == "" || t.Version <= 0 {
		return ErrInvalidEntity
	}
	if !t.Status.IsValid() {
		return ErrInvalidEntity
	}
	return nil
}

func (c *NotificationCampaign) Validate(ctx context.Context) error {
	if c.ID == "" || c.Name == "" || c.TemplateID == "" || c.CampaignType == "" {
		return ErrInvalidEntity
	}
	if !c.Status.IsValid() {
		return ErrInvalidEntity
	}
	return nil
}

func (r *CampaignRecipient) Validate(ctx context.Context) error {
	if r.ID == "" || r.CampaignID == "" || r.RecipientEmail == "" {
		return ErrInvalidEntity
	}
	if !r.DeliveryStatus.IsValid() {
		return ErrInvalidEntity
	}
	return nil
}

func (n *DirectNotification) Validate(ctx context.Context) error {
	if n.ID == "" || n.TemplateID == "" || n.RecipientEmail == "" {
		return ErrInvalidEntity
	}
	if !n.DeliveryStatus.IsValid() {
		return ErrInvalidEntity
	}
	if !n.NotificationType.IsValid() {
		return ErrInvalidEntity
	}
	return nil
}

func (r *AutomationRule) Validate(ctx context.Context) error {
	if r.ID == "" || r.Name == "" || r.EventType == "" || r.TemplateID == "" {
		return ErrInvalidEntity
	}
	return nil
}

type EmailTemplate struct {
	ID            string         `json:"id" db:"id"`
	TemplateKey   string         `json:"templateKey" db:"template_key"`
	Name          string         `json:"name" db:"name"`
	Subject       string         `json:"subject" db:"subject"`
	HTMLBody      string         `json:"htmlBody" db:"html_body"`
	PlainTextBody string         `json:"plainTextBody,omitempty" db:"plain_text_body"`
	Locale        string         `json:"locale" db:"locale"`
	Status        TemplateStatus `json:"status" db:"status"`
	Version       int            `json:"version" db:"version"`
	CreatedAt     time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time      `json:"updatedAt" db:"updated_at"`
}

type NotificationCampaign struct {
	ID           string         `json:"id" db:"id"`
	Name         string         `json:"name" db:"name"`
	TemplateID   string         `json:"templateId" db:"template_id"`
	CampaignType string         `json:"campaignType" db:"campaign_type"`
	Status       CampaignStatus `json:"status" db:"status"`
	ScheduledAt  time.Time      `json:"scheduledAt" db:"scheduled_at"`
	StartedAt    *time.Time     `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty" db:"completed_at"`
	CreatedAt    time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time      `json:"updatedAt" db:"updated_at"`
}

type CampaignRecipient struct {
	ID             string         `json:"id" db:"id"`
	CampaignID     string         `json:"campaignId" db:"campaign_id"`
	ExternalUserID string         `json:"externalUserId,omitempty" db:"external_user_id"`
	RecipientEmail string         `json:"recipientEmail" db:"recipient_email"`
	RecipientName  string         `json:"recipientName,omitempty" db:"recipient_name"`
	DeliveryStatus DeliveryStatus `json:"deliveryStatus" db:"delivery_status"`
	AttemptsCount  int            `json:"attemptsCount" db:"attempts_count"`
	LastAttemptAt  *time.Time     `json:"lastAttemptAt,omitempty" db:"last_attempt_at"`
	SentAt         *time.Time     `json:"sentAt,omitempty" db:"sent_at"`
	ErrorMessage   string         `json:"errorMessage,omitempty" db:"error_message"`
	CreatedAt      time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time      `json:"updatedAt" db:"updated_at"`
}

type DirectNotification struct {
	ID               string           `json:"id" db:"id"`
	TemplateID       string           `json:"templateId" db:"template_id"`
	ExternalUserID   string           `json:"externalUserId,omitempty" db:"external_user_id"`
	RecipientEmail   string           `json:"recipientEmail" db:"recipient_email"`
	RecipientName    string           `json:"recipientName,omitempty" db:"recipient_name"`
	NotificationType NotificationType `json:"notificationType" db:"notification_type"`
	DeliveryStatus   DeliveryStatus   `json:"deliveryStatus" db:"delivery_status"`
	AttemptsCount    int              `json:"attemptsCount" db:"attempts_count"`
	LastAttemptAt    *time.Time       `json:"lastAttemptAt,omitempty" db:"last_attempt_at"`
	SentAt           *time.Time       `json:"sentAt,omitempty" db:"sent_at"`
	ErrorMessage     string           `json:"errorMessage,omitempty" db:"error_message"`
	Payload          json.RawMessage  `json:"payload,omitempty" db:"payload"`
	CreatedAt        time.Time        `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time        `json:"updatedAt" db:"updated_at"`
}

type DeliveryAttempt struct {
	ID           string             `json:"id" db:"id"`
	TargetType   DeliveryTargetType `json:"targetType" db:"target_type"`
	TargetID     string             `json:"targetId" db:"target_id"`
	Status       DeliveryStatus     `json:"status" db:"status"`
	AttemptNumber int               `json:"attemptNumber" db:"attempt_number"`
	ErrorMessage string             `json:"errorMessage,omitempty" db:"error_message"`
	AttemptedAt  time.Time          `json:"attemptedAt" db:"attempted_at"`
	CreatedAt    time.Time          `json:"createdAt" db:"created_at"`
}

type AutomationRule struct {
	ID         string          `json:"id" db:"id"`
	Name       string          `json:"name" db:"name"`
	EventType  string          `json:"eventType" db:"event_type"`
	TemplateID string          `json:"templateId" db:"template_id"`
	Enabled    bool            `json:"enabled" db:"enabled"`
	Config     json.RawMessage `json:"config" db:"config"`
	CreatedAt  time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time       `json:"updatedAt" db:"updated_at"`
}

type AppSetting struct {
	Key         string    `json:"key" db:"key"`
	Value       string    `json:"value" db:"value"`
	Category    string    `json:"category" db:"category"`
	Description string    `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}
