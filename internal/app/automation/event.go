package automation

import (
	"time"

	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

// Supported event type constants (standard event identifiers)
const (
	EventUserRegistered      = "user.registered"
	EventUserInactive        = "user.inactive"
	EventEmailConfirmed      = "email.confirmed"
	EventPasswordChanged     = "password.changed"
	EventUserBlocked         = "user.blocked"
	EventUserUnblocked       = "user.unblocked"
	EventUserLanguageChanged = "user.language_changed"
)

// Event is the transport-agnostic internal runtime event representation.
type Event struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	UserID     string           `json:"userId"`
	User       *userclient.User `json:"user,omitempty"`
	Data       map[string]any   `json:"data,omitempty"`
	OccurredAt time.Time        `json:"occurredAt"`
}

// ExecutionStatus represents the outcome of evaluating and executing a rule against an event.
type ExecutionStatus string

const (
	StatusExecuted          ExecutionStatus = "executed"
	StatusSkippedNotMatched ExecutionStatus = "skipped_not_matched"
	StatusSkippedCooldown   ExecutionStatus = "skipped_cooldown"
	StatusSkippedDuplicate  ExecutionStatus = "skipped_duplicate"
	StatusFailed            ExecutionStatus = "failed"
)

// RuleExecutionResult represents the execution result for an individual automation rule.
type RuleExecutionResult struct {
	RuleID         string          `json:"ruleId"`
	RuleName       string          `json:"ruleName"`
	Status         ExecutionStatus `json:"status"`
	NotificationID *string         `json:"notificationId,omitempty"`
	Error          string          `json:"error,omitempty"`
}

// EventExecutionResult represents the overall batch result of processing an event across all rules.
type EventExecutionResult struct {
	EventID   string                `json:"eventId"`
	EventType string                `json:"eventType"`
	Results   []RuleExecutionResult `json:"results"`
}
