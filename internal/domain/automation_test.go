package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func intPtr(i int) *int {
	return &i
}

func TestAutomationRuleConfig_ValidMinimalRule(t *testing.T) {
	cfg := &AutomationRuleConfig{
		Version: 1,
		Schedule: ScheduleConfig{
			Type:      ScheduleTypeDaily,
			HourUTC:   intPtr(3),
			MinuteUTC: intPtr(0),
		},
		Conditions: ConditionGroup{
			Operator: GroupOperatorAll,
			Conditions: []ConditionNode{
				{
					Item: &ConditionItem{
						Field:    FieldIsBlocked,
						Operator: OperatorEquals,
						Value:    false,
					},
				},
			},
		},
		Action: ActionConfig{
			Type:         ActionTypeSendEmail,
			CooldownDays: intPtr(90),
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestAutomationRuleConfig_MultipleAndNestedConditions(t *testing.T) {
	cfg := &AutomationRuleConfig{
		Version: 1,
		Schedule: ScheduleConfig{
			Type:      ScheduleTypeWeekly,
			HourUTC:   intPtr(12),
			MinuteUTC: intPtr(30),
			DayOfWeek: intPtr(1),
		},
		Conditions: ConditionGroup{
			Operator: GroupOperatorAll,
			Conditions: []ConditionNode{
				{
					Item: &ConditionItem{
						Field:    FieldIsBlocked,
						Operator: OperatorEquals,
						Value:    false,
					},
				},
				{
					Item: &ConditionItem{
						Field:    FieldIsActive,
						Operator: OperatorEquals,
						Value:    true,
					},
				},
				{
					Item: &ConditionItem{
						Field:    FieldEmail,
						Operator: OperatorExists,
					},
				},
				{
					Item: &ConditionItem{
						Field:    FieldRegistrationDate,
						Operator: OperatorOlderThan,
						Value:    365.0,
						Unit:     UnitDays,
					},
				},
				{
					Group: &ConditionGroup{
						Operator: GroupOperatorAny,
						Conditions: []ConditionNode{
							{
								Item: &ConditionItem{
									Field:    FieldLanguage,
									Operator: OperatorEquals,
									Value:    "EN",
								},
							},
							{
								Item: &ConditionItem{
									Field:    FieldLanguage,
									Operator: OperatorIn,
									Value:    []any{"UA", "PL"},
								},
							},
						},
					},
				},
			},
		},
		Action: ActionConfig{
			Type:         ActionTypeSendEmail,
			CooldownDays: intPtr(0),
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid nested config, got error: %v", err)
	}
}

func TestAutomationRuleConfig_JSONSerialization(t *testing.T) {
	rawJSON := `{
		"version": 1,
		"schedule": {
			"type": "interval_hours",
			"intervalHours": 24
		},
		"conditions": {
			"operator": "all",
			"conditions": [
				{
					"field": "role",
					"operator": "not_in",
					"value": ["guest", "banned"]
				},
				{
					"operator": "any",
					"conditions": [
						{
							"field": "username",
							"operator": "starts_with",
							"value": "vip_"
						},
						{
							"field": "registrationDate",
							"operator": "before",
							"value": "2025-01-01T00:00:00Z"
						}
					]
				}
			]
		},
		"action": {
			"type": "send_email",
			"cooldownDays": 14
		}
	}`

	var cfg AutomationRuleConfig
	if err := json.Unmarshal([]byte(rawJSON), &cfg); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unmarshaled config failed validation: %v", err)
	}

	if len(cfg.Conditions.Conditions) != 2 {
		t.Fatalf("expected 2 root conditions, got %d", len(cfg.Conditions.Conditions))
	}

	if cfg.Conditions.Conditions[0].Item == nil || cfg.Conditions.Conditions[0].Item.Field != "role" {
		t.Errorf("expected first node to be item for 'role'")
	}

	if cfg.Conditions.Conditions[1].Group == nil || cfg.Conditions.Conditions[1].Group.Operator != "any" {
		t.Errorf("expected second node to be group with operator 'any'")
	}

	// Test Marshal back to JSON
	marshaled, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	var roundtrip AutomationRuleConfig
	if err := json.Unmarshal(marshaled, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal marshaled config: %v", err)
	}
	if err := roundtrip.Validate(); err != nil {
		t.Fatalf("roundtrip config failed validation: %v", err)
	}
}

func TestAutomationRuleConfig_ValidationFailures(t *testing.T) {
	validSchedule := ScheduleConfig{
		Type:      ScheduleTypeDaily,
		HourUTC:   intPtr(4),
		MinuteUTC: intPtr(30),
	}
	validAction := ActionConfig{
		Type: ActionTypeSendEmail,
	}

	tests := []struct {
		name string
		cfg  AutomationRuleConfig
	}{
		{
			name: "unsupported version 0",
			cfg: AutomationRuleConfig{
				Version:  0,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "unsupported version 2",
			cfg: AutomationRuleConfig{
				Version:  2,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "invalid schedule type",
			cfg: AutomationRuleConfig{
				Version: 1,
				Schedule: ScheduleConfig{
					Type: "hourly_at_minute",
				},
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "daily schedule missing hour",
			cfg: AutomationRuleConfig{
				Version: 1,
				Schedule: ScheduleConfig{
					Type:      ScheduleTypeDaily,
					MinuteUTC: intPtr(10),
				},
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "daily schedule hour out of range",
			cfg: AutomationRuleConfig{
				Version: 1,
				Schedule: ScheduleConfig{
					Type:      ScheduleTypeDaily,
					HourUTC:   intPtr(24),
					MinuteUTC: intPtr(0),
				},
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "weekly schedule dayOfWeek out of range",
			cfg: AutomationRuleConfig{
				Version: 1,
				Schedule: ScheduleConfig{
					Type:      ScheduleTypeWeekly,
					HourUTC:   intPtr(10),
					MinuteUTC: intPtr(0),
					DayOfWeek: intPtr(7),
				},
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "interval_hours out of range",
			cfg: AutomationRuleConfig{
				Version: 1,
				Schedule: ScheduleConfig{
					Type:          ScheduleTypeIntervalHours,
					IntervalHours: intPtr(200),
				},
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "empty condition group",
			cfg: AutomationRuleConfig{
				Version:    1,
				Schedule:   validSchedule,
				Conditions: ConditionGroup{Operator: "all", Conditions: []ConditionNode{}},
				Action:     validAction,
			},
		},
		{
			name: "invalid group operator",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "none",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "invalid field name (unsupported field / lastLoginAt excluded in v1)",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: "lastLoginAt", Operator: OperatorOlderThan, Value: 30, Unit: "days"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "boolean field with text operator (isBlocked + contains)",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsBlocked, Operator: OperatorContains, Value: "true"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "boolean field with non-bool value (isBlocked + equals string)",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsBlocked, Operator: OperatorEquals, Value: "false"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "enum field with date operator (language + older_than)",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldLanguage, Operator: OperatorOlderThan, Value: 10, Unit: "days"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "enum field with in operator but non-array value",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldLanguage, Operator: OperatorIn, Value: "EN"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "enum field with in operator but empty array",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldLanguage, Operator: OperatorIn, Value: []any{}}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "text field with older_than (email + older_than)",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldEmail, Operator: OperatorOlderThan, Value: 10, Unit: "days"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "date field older_than without unit",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldRegistrationDate, Operator: OperatorOlderThan, Value: 10}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "date field older_than with invalid unit",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldRegistrationDate, Operator: OperatorOlderThan, Value: 10, Unit: "weeks"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "date field older_than with non-positive number",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldRegistrationDate, Operator: OperatorOlderThan, Value: -5, Unit: "days"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "date field before with invalid date format",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldRegistrationDate, Operator: OperatorBefore, Value: "invalid-date-format"}},
					},
				},
				Action: validAction,
			},
		},
		{
			name: "invalid action type",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: ActionConfig{Type: "webhook"},
			},
		},
		{
			name: "invalid negative cooldownDays",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{Item: &ConditionItem{Field: FieldIsActive, Operator: OperatorEquals, Value: true}},
					},
				},
				Action: ActionConfig{
					Type:         ActionTypeSendEmail,
					CooldownDays: intPtr(-1),
				},
			},
		},
		{
			name: "nested group is empty",
			cfg: AutomationRuleConfig{
				Version:  1,
				Schedule: validSchedule,
				Conditions: ConditionGroup{
					Operator: "all",
					Conditions: []ConditionNode{
						{
							Group: &ConditionGroup{
								Operator:   "any",
								Conditions: []ConditionNode{},
							},
						},
					},
				},
				Action: validAction,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Errorf("expected error for case %q, got nil", tt.name)
			}
			if !errors.Is(err, ErrInvalidEntity) {
				t.Errorf("expected ErrInvalidEntity, got %v", err)
			}
		})
	}
}
