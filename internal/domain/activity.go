package domain

import (
	"context"
	"encoding/json"
	"time"
)

type ActivityActorType string
type ActivityStatus string
type ActivityTargetType string

const (
	ActorTypeUser    ActivityActorType = "user"
	ActorTypeSystem  ActivityActorType = "system"
	ActorTypeService ActivityActorType = "service"
)

const (
	ActivityStatusSuccess ActivityStatus = "success"
	ActivityStatusFailure ActivityStatus = "failure"
	ActivityStatusWarning ActivityStatus = "warning"
)

const (
	TargetTypeCampaign           ActivityTargetType = "campaign"
	TargetTypeDirectNotification ActivityTargetType = "direct_notification"
	TargetTypeTemplate           ActivityTargetType = "template"
	TargetTypeAutomationRule     ActivityTargetType = "automation_rule"
	TargetTypeSettings           ActivityTargetType = "settings"
	TargetTypeAgreement          ActivityTargetType = "agreement"
)

const (
	// Direct Notification Events
	EventDirectCreated   = "direct.created"
	EventDirectDelivered = "direct.delivered"
	EventDirectFailed    = "direct.failed"

	// Campaign Events
	EventCampaignCreated   = "campaign.created"
	EventCampaignScheduled = "campaign.scheduled"
	EventCampaignStarted   = "campaign.started"
	EventCampaignCancelled = "campaign.cancelled"
	EventCampaignCompleted = "campaign.completed"
	EventCampaignFailed    = "campaign.failed"

	// Agreement Events
	EventAgreementBroadcastCreated = "agreement.broadcast_created"

	// Template Events
	EventTemplateCreated       = "template.created"
	EventTemplateUpdated       = "template.updated"
	EventTemplateStatusChanged = "template.status_changed"
	EventTemplateArchived      = "template.archived"

	// Automation Events
	EventAutomationCreated  = "automation.created"
	EventAutomationUpdated  = "automation.updated"
	EventAutomationEnabled  = "automation.enabled"
	EventAutomationDisabled = "automation.disabled"
	EventAutomationDeleted  = "automation.deleted"
	EventAutomationExecuted = "automation.executed"
	EventAutomationFailed   = "automation.failed"

	// Settings Events
	EventSettingsUpdated = "settings.updated"
)

func (t ActivityActorType) IsValid() bool {
	switch t {
	case ActorTypeUser, ActorTypeSystem, ActorTypeService:
		return true
	default:
		return false
	}
}

func (s ActivityStatus) IsValid() bool {
	switch s {
	case ActivityStatusSuccess, ActivityStatusFailure, ActivityStatusWarning:
		return true
	default:
		return false
	}
}

func (t ActivityTargetType) IsValid() bool {
	switch t {
	case TargetTypeCampaign, TargetTypeDirectNotification, TargetTypeTemplate,
		TargetTypeAutomationRule, TargetTypeSettings, TargetTypeAgreement:
		return true
	default:
		return false
	}
}

// ActivityLog represents an immutable audit log record for an administrative or business event.
type ActivityLog struct {
	ID           string             `json:"id" db:"id"`
	EventType    string             `json:"eventType" db:"event_type"`
	ActorType    ActivityActorType  `json:"actorType" db:"actor_type"`
	ActorUserID  *string            `json:"actorUserId,omitempty" db:"actor_user_id"`
	ActorName    *string            `json:"actorName,omitempty" db:"actor_name"`
	ActorRole    *string            `json:"actorRole,omitempty" db:"actor_role"`
	TargetType   ActivityTargetType `json:"targetType" db:"target_type"`
	TargetID     string             `json:"targetId" db:"target_id"`
	TargetName   *string            `json:"targetName,omitempty" db:"target_name"`
	Status       ActivityStatus     `json:"status" db:"status"`
	Summary      string             `json:"summary" db:"summary"`
	Details      json.RawMessage    `json:"details,omitempty" db:"details"`
	ErrorMessage *string            `json:"errorMessage,omitempty" db:"error_message"`
	CreatedAt    time.Time          `json:"createdAt" db:"created_at"`
}

func (a *ActivityLog) Validate(ctx context.Context) error {
	if a.ID == "" || a.EventType == "" || a.Summary == "" || a.TargetID == "" {
		return ErrInvalidEntity
	}
	if !a.ActorType.IsValid() || !a.TargetType.IsValid() || !a.Status.IsValid() {
		return ErrInvalidEntity
	}
	if len(a.Details) > 0 {
		var js any
		if err := json.Unmarshal(a.Details, &js); err != nil {
			return ErrInvalidEntity
		}
	}
	return nil
}
