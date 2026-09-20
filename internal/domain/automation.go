package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Schedule types
const (
	ScheduleTypeDaily         = "daily"
	ScheduleTypeWeekly        = "weekly"
	ScheduleTypeIntervalHours = "interval_hours"
)

// Group operators
const (
	GroupOperatorAll = "all"
	GroupOperatorAny = "any"
)

// Condition operators
const (
	// Equality & Set
	OperatorEquals    = "equals"
	OperatorNotEquals = "not_equals"
	OperatorIn        = "in"
	OperatorNotIn     = "not_in"

	// Text
	OperatorContains   = "contains"
	OperatorStartsWith = "starts_with"
	OperatorEndsWith   = "ends_with"
	OperatorExists     = "exists"
	OperatorNotExists  = "not_exists"

	// Date / Time
	OperatorOlderThan = "older_than"
	OperatorNewerThan = "newer_than"
	OperatorBefore    = "before"
	OperatorAfter     = "after"
)

// Relative time units
const (
	UnitDays   = "days"
	UnitMonths = "months"
	UnitYears  = "years"
)

// Action types
const (
	ActionTypeSendEmail = "send_email"
)

// Supported trigger types
const (
	TriggerUserRegistered      = "user.registered"
	TriggerEmailConfirmed      = "email.confirmed"
	TriggerPasswordChanged     = "password.changed"
	TriggerUserBlocked         = "user.blocked"
	TriggerUserUnblocked       = "user.unblocked"
	TriggerUserLanguageChanged = "user.language_changed"
	TriggerUserInactive        = "user.inactive"
)

// SupportedTriggers contains all canonical trigger identifiers
var SupportedTriggers = []string{
	TriggerUserRegistered,
	TriggerEmailConfirmed,
	TriggerPasswordChanged,
	TriggerUserBlocked,
	TriggerUserUnblocked,
	TriggerUserLanguageChanged,
	TriggerUserInactive,
}

// IsSupportedTrigger checks whether a given trigger type is supported
func IsSupportedTrigger(trigger string) bool {
	t := strings.ToLower(strings.TrimSpace(trigger))
	for _, st := range SupportedTriggers {
		if st == t {
			return true
		}
	}
	return false
}

// Supported condition field names in v1
const (
	FieldIsBlocked        = "isBlocked"
	FieldIsActive         = "isActive"
	FieldEmail            = "email"
	FieldLanguage         = "language"
	FieldRole             = "role"
	FieldUsername         = "username"
	FieldRegistrationDate = "registrationDate"
	FieldLastActivityAt   = "lastActivityAt"
	FieldLastActivity     = "lastActivity"
)

// AutomationRuleConfig is the root configuration structure.
type AutomationRuleConfig struct {
	Version    int            `json:"version"`
	Trigger    string         `json:"trigger"`
	Schedule   ScheduleConfig `json:"schedule"`
	Conditions ConditionGroup `json:"conditions"`
	Action     ActionConfig   `json:"action"`
}

// Value implements driver.Valuer for database/sql JSON serialization.
func (c AutomationRuleConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner for database/sql JSON deserialization.
func (c *AutomationRuleConfig) Scan(src any) error {
	if src == nil {
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into AutomationRuleConfig", src)
	}
	return json.Unmarshal(b, c)
}

// ScheduleConfig holds evaluation timing parameters.
type ScheduleConfig struct {
	Type          string `json:"type"`
	HourUTC       *int   `json:"hourUtc,omitempty"`
	MinuteUTC     *int   `json:"minuteUtc,omitempty"`
	DayOfWeek     *int   `json:"dayOfWeek,omitempty"`     // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
	IntervalHours *int   `json:"intervalHours,omitempty"` // 1..168
}

// ConditionGroup defines a logical grouping (ALL or ANY) of conditions.
type ConditionGroup struct {
	Operator   string          `json:"operator"` // "all" | "any"
	Conditions []ConditionNode `json:"conditions"`
}

// ConditionNode represents either a single ConditionItem or a nested ConditionGroup.
type ConditionNode struct {
	Item  *ConditionItem  `json:"-"`
	Group *ConditionGroup `json:"-"`
}

func (n ConditionNode) MarshalJSON() ([]byte, error) {
	if n.Group != nil {
		return json.Marshal(n.Group)
	}
	if n.Item != nil {
		return json.Marshal(n.Item)
	}
	return json.Marshal(nil)
}

func (n *ConditionNode) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// If it contains "field", it's a leaf ConditionItem.
	if _, ok := raw["field"]; ok {
		var item ConditionItem
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		n.Item = &item
		n.Group = nil
		return nil
	}

	// If it contains "conditions" or ("operator" with "all"/"any" and no "field"), it's a ConditionGroup.
	if _, ok := raw["conditions"]; ok {
		var group ConditionGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return err
		}
		n.Group = &group
		n.Item = nil
		return nil
	}

	// Fallback to checking if operator is all/any
	if opRaw, ok := raw["operator"]; ok {
		var opStr string
		if err := json.Unmarshal(opRaw, &opStr); err == nil {
			if strings.EqualFold(opStr, GroupOperatorAll) || strings.EqualFold(opStr, GroupOperatorAny) {
				var group ConditionGroup
				if err := json.Unmarshal(data, &group); err != nil {
					return err
				}
				n.Group = &group
				n.Item = nil
				return nil
			}
		}
	}

	// Try unmarshaling as ConditionItem by default
	var item ConditionItem
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	n.Item = &item
	n.Group = nil
	return nil
}

// ConditionItem is an atomic condition applied to a user field.
type ConditionItem struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	Unit     string `json:"unit,omitempty"` // "days" | "months" | "years"
}

// ActionConfig defines the execution action.
type ActionConfig struct {
	Type         string `json:"type"` // "send_email"
	CooldownDays *int   `json:"cooldownDays,omitempty"`
}

// Validate validates the entire AutomationRuleConfig AST.
func (c *AutomationRuleConfig) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("%w: config version %d is not supported (expected 1)", ErrInvalidEntity, c.Version)
	}

	trigger := strings.ToLower(strings.TrimSpace(c.Trigger))
	if trigger == "" {
		return fmt.Errorf("%w: automation rule trigger is required", ErrInvalidEntity)
	}
	if !IsSupportedTrigger(trigger) {
		return fmt.Errorf("%w: unsupported or invalid automation trigger %q", ErrInvalidEntity, c.Trigger)
	}

	if err := c.Schedule.Validate(); err != nil {
		return err
	}

	if err := c.Conditions.Validate(); err != nil {
		return err
	}

	if err := c.Action.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate validates ScheduleConfig.
func (s *ScheduleConfig) Validate() error {
	switch s.Type {
	case ScheduleTypeDaily:
		if s.HourUTC == nil || *s.HourUTC < 0 || *s.HourUTC > 23 {
			return fmt.Errorf("%w: daily schedule hourUtc must be between 0 and 23", ErrInvalidEntity)
		}
		if s.MinuteUTC == nil || *s.MinuteUTC < 0 || *s.MinuteUTC > 59 {
			return fmt.Errorf("%w: daily schedule minuteUtc must be between 0 and 59", ErrInvalidEntity)
		}
	case ScheduleTypeWeekly:
		if s.HourUTC == nil || *s.HourUTC < 0 || *s.HourUTC > 23 {
			return fmt.Errorf("%w: weekly schedule hourUtc must be between 0 and 23", ErrInvalidEntity)
		}
		if s.MinuteUTC == nil || *s.MinuteUTC < 0 || *s.MinuteUTC > 59 {
			return fmt.Errorf("%w: weekly schedule minuteUtc must be between 0 and 59", ErrInvalidEntity)
		}
		if s.DayOfWeek == nil || *s.DayOfWeek < 0 || *s.DayOfWeek > 6 {
			return fmt.Errorf("%w: weekly schedule dayOfWeek must be between 0 and 6", ErrInvalidEntity)
		}
	case ScheduleTypeIntervalHours:
		if s.IntervalHours == nil || *s.IntervalHours < 1 || *s.IntervalHours > 168 {
			return fmt.Errorf("%w: interval_hours schedule intervalHours must be between 1 and 168", ErrInvalidEntity)
		}
	default:
		return fmt.Errorf("%w: invalid or unsupported schedule type %q", ErrInvalidEntity, s.Type)
	}
	return nil
}

// Validate validates ConditionGroup recursively.
func (g *ConditionGroup) Validate() error {
	op := strings.ToLower(strings.TrimSpace(g.Operator))
	if op != GroupOperatorAll && op != GroupOperatorAny {
		return fmt.Errorf("%w: group operator must be %q or %q, got %q", ErrInvalidEntity, GroupOperatorAll, GroupOperatorAny, g.Operator)
	}

	if len(g.Conditions) == 0 {
		return fmt.Errorf("%w: condition group must contain at least one condition", ErrInvalidEntity)
	}

	for i, node := range g.Conditions {
		if node.Group != nil && node.Item != nil {
			return fmt.Errorf("%w: condition node [%d] cannot be both a group and an item", ErrInvalidEntity, i)
		}
		if node.Group == nil && node.Item == nil {
			return fmt.Errorf("%w: condition node [%d] is empty", ErrInvalidEntity, i)
		}
		if node.Group != nil {
			if err := node.Group.Validate(); err != nil {
				return err
			}
		}
		if node.Item != nil {
			if err := node.Item.Validate(); err != nil {
				return err
			}
		}
	}

	return nil
}

// Validate validates ConditionItem against the field/operator/value matrix.
func (item *ConditionItem) Validate() error {
	field := strings.TrimSpace(item.Field)
	operator := strings.ToLower(strings.TrimSpace(item.Operator))

	if field == "" {
		return fmt.Errorf("%w: condition field is required", ErrInvalidEntity)
	}
	if operator == "" {
		return fmt.Errorf("%w: condition operator is required for field %q", ErrInvalidEntity, field)
	}

	switch field {
	case FieldIsBlocked, FieldIsActive:
		return validateBooleanField(field, operator, item.Value)

	case FieldLanguage, FieldRole:
		return validateEnumField(field, operator, item.Value)

	case FieldEmail, FieldUsername:
		return validateTextField(field, operator, item.Value)

	case FieldRegistrationDate, FieldLastActivityAt, FieldLastActivity:
		return validateDateField(field, operator, item.Value, item.Unit)

	default:
		return fmt.Errorf("%w: unsupported or invalid condition field %q", ErrInvalidEntity, field)
	}
}

func validateBooleanField(field, operator string, val any) error {
	switch operator {
	case OperatorEquals, OperatorNotEquals:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("%w: field %q with operator %q requires a boolean value", ErrInvalidEntity, field, operator)
		}
		return nil
	default:
		return fmt.Errorf("%w: operator %q is not valid for boolean field %q", ErrInvalidEntity, operator, field)
	}
}

func validateEnumField(field, operator string, val any) error {
	switch operator {
	case OperatorEquals, OperatorNotEquals:
		str, ok := val.(string)
		if !ok || strings.TrimSpace(str) == "" {
			return fmt.Errorf("%w: field %q with operator %q requires a non-empty string value", ErrInvalidEntity, field, operator)
		}
		return nil

	case OperatorIn, OperatorNotIn:
		slice, ok := val.([]any)
		if !ok {
			// Check if []string
			if strSlice, okStr := val.([]string); okStr {
				if len(strSlice) == 0 {
					return fmt.Errorf("%w: field %q with operator %q requires a non-empty list of strings", ErrInvalidEntity, field, operator)
				}
				for _, s := range strSlice {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%w: field %q list elements must not be empty", ErrInvalidEntity, field)
					}
				}
				return nil
			}
			return fmt.Errorf("%w: field %q with operator %q requires an array value", ErrInvalidEntity, field, operator)
		}
		if len(slice) == 0 {
			return fmt.Errorf("%w: field %q with operator %q requires a non-empty array", ErrInvalidEntity, field, operator)
		}
		for _, elem := range slice {
			s, isStr := elem.(string)
			if !isStr || strings.TrimSpace(s) == "" {
				return fmt.Errorf("%w: field %q with operator %q elements must be non-empty strings", ErrInvalidEntity, field, operator)
			}
		}
		return nil

	default:
		return fmt.Errorf("%w: operator %q is not valid for enum field %q", ErrInvalidEntity, operator, field)
	}
}

func validateTextField(field, operator string, val any) error {
	switch operator {
	case OperatorExists, OperatorNotExists:
		// No value required for existence check
		return nil

	case OperatorEquals, OperatorNotEquals, OperatorContains, OperatorStartsWith, OperatorEndsWith:
		str, ok := val.(string)
		if !ok || strings.TrimSpace(str) == "" {
			return fmt.Errorf("%w: field %q with operator %q requires a non-empty string value", ErrInvalidEntity, field, operator)
		}
		return nil

	default:
		return fmt.Errorf("%w: operator %q is not valid for text field %q", ErrInvalidEntity, operator, field)
	}
}

func validateDateField(field, operator string, val any, unit string) error {
	switch operator {
	case OperatorOlderThan, OperatorNewerThan:
		// Numeric positive duration + unit
		var num float64
		switch v := val.(type) {
		case float64:
			num = v
		case int:
			num = float64(v)
		case int64:
			num = float64(v)
		default:
			return fmt.Errorf("%w: field %q with operator %q requires a positive numeric value", ErrInvalidEntity, field, operator)
		}
		if num <= 0 {
			return fmt.Errorf("%w: field %q with operator %q requires a positive numeric value, got %v", ErrInvalidEntity, field, operator, num)
		}

		u := strings.ToLower(strings.TrimSpace(unit))
		switch u {
		case UnitDays, UnitMonths, UnitYears:
			return nil
		default:
			return fmt.Errorf("%w: field %q with operator %q requires a valid unit (%s, %s, or %s), got %q",
				ErrInvalidEntity, field, operator, UnitDays, UnitMonths, UnitYears, unit)
		}

	case OperatorBefore, OperatorAfter:
		// ISO 8601 / RFC 3339 date string
		str, ok := val.(string)
		if !ok || strings.TrimSpace(str) == "" {
			return fmt.Errorf("%w: field %q with operator %q requires a date/time string (RFC3339 or YYYY-MM-DD)", ErrInvalidEntity, field, operator)
		}
		str = strings.TrimSpace(str)
		if _, err := time.Parse(time.RFC3339, str); err != nil {
			if _, err2 := time.Parse("2006-01-02", str); err2 != nil {
				return fmt.Errorf("%w: field %q with operator %q has invalid date format %q", ErrInvalidEntity, field, operator, str)
			}
		}
		return nil

	default:
		return fmt.Errorf("%w: operator %q is not valid for date field %q", ErrInvalidEntity, operator, field)
	}
}

// Validate validates ActionConfig.
func (a *ActionConfig) Validate() error {
	if a.Type != ActionTypeSendEmail {
		return fmt.Errorf("%w: invalid or unsupported action type %q (expected %q)", ErrInvalidEntity, a.Type, ActionTypeSendEmail)
	}

	if a.CooldownDays != nil && *a.CooldownDays < 0 {
		return fmt.Errorf("%w: action cooldownDays must be >= 0", ErrInvalidEntity)
	}

	return nil
}
