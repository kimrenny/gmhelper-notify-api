package automation

import (
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

func TestEvaluator_BooleanFields(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Now().UTC()

	tests := []struct {
		name     string
		item     domain.ConditionItem
		ctx      map[string]any
		expected bool
	}{
		{
			name: "isBlocked equals true -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldIsBlocked,
				Operator: domain.OperatorEquals,
				Value:    true,
			},
			ctx:      map[string]any{"isBlocked": true},
			expected: true,
		},
		{
			name: "isBlocked equals true when false -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldIsBlocked,
				Operator: domain.OperatorEquals,
				Value:    true,
			},
			ctx:      map[string]any{"isBlocked": false},
			expected: false,
		},
		{
			name: "isActive not_equals false when true -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldIsActive,
				Operator: domain.OperatorNotEquals,
				Value:    false,
			},
			ctx:      map[string]any{"isActive": true},
			expected: true,
		},
		{
			name: "missing boolean field -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldIsBlocked,
				Operator: domain.OperatorEquals,
				Value:    false,
			},
			ctx:      map[string]any{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			group := domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &tc.item},
				},
			}
			matched, err := eval.Evaluate(group, tc.ctx, refTime)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, matched)
			}
		})
	}
}

func TestEvaluator_EnumFields(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Now().UTC()

	tests := []struct {
		name     string
		item     domain.ConditionItem
		ctx      map[string]any
		expected bool
	}{
		{
			name: "role equals Admin (case insensitive) -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRole,
				Operator: domain.OperatorEquals,
				Value:    "admin",
			},
			ctx:      map[string]any{"role": "Admin"},
			expected: true,
		},
		{
			name: "role not_equals Owner when Admin -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRole,
				Operator: domain.OperatorNotEquals,
				Value:    "owner",
			},
			ctx:      map[string]any{"role": "Admin"},
			expected: true,
		},
		{
			name: "language in [en, de] when EN -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldLanguage,
				Operator: domain.OperatorIn,
				Value:    []string{"en", "de"},
			},
			ctx:      map[string]any{"language": "EN"},
			expected: true,
		},
		{
			name: "language in [en, de] when fr -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldLanguage,
				Operator: domain.OperatorIn,
				Value:    []any{"en", "de"},
			},
			ctx:      map[string]any{"language": "fr"},
			expected: false,
		},
		{
			name: "language not_in [ru, zh] when en -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldLanguage,
				Operator: domain.OperatorNotIn,
				Value:    []string{"ru", "zh"},
			},
			ctx:      map[string]any{"language": "en"},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			group := domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &tc.item},
				},
			}
			matched, err := eval.Evaluate(group, tc.ctx, refTime)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, matched)
			}
		})
	}
}

func TestEvaluator_TextFields(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Now().UTC()

	tests := []struct {
		name     string
		item     domain.ConditionItem
		ctx      map[string]any
		expected bool
	}{
		{
			name: "email contains @company.com -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldEmail,
				Operator: domain.OperatorContains,
				Value:    "@company.com",
			},
			ctx:      map[string]any{"email": "alice@company.com"},
			expected: true,
		},
		{
			name: "username starts_with test_ -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldUsername,
				Operator: domain.OperatorStartsWith,
				Value:    "test_",
			},
			ctx:      map[string]any{"username": "test_user_42"},
			expected: true,
		},
		{
			name: "username ends_with _vip -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldUsername,
				Operator: domain.OperatorEndsWith,
				Value:    "_vip",
			},
			ctx:      map[string]any{"username": "john_doe_VIP"},
			expected: true,
		},
		{
			name: "email exists when present -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldEmail,
				Operator: domain.OperatorExists,
			},
			ctx:      map[string]any{"email": "bob@example.com"},
			expected: true,
		},
		{
			name: "email exists when empty string -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldEmail,
				Operator: domain.OperatorExists,
			},
			ctx:      map[string]any{"email": ""},
			expected: false,
		},
		{
			name: "username not_exists when missing -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldUsername,
				Operator: domain.OperatorNotExists,
			},
			ctx:      map[string]any{},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			group := domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &tc.item},
				},
			}
			matched, err := eval.Evaluate(group, tc.ctx, refTime)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, matched)
			}
		})
	}
}

func TestEvaluator_DateFields(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		item     domain.ConditionItem
		ctx      map[string]any
		expected bool
	}{
		{
			name: "registered 40 days ago is older_than 30 days -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRegistrationDate,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"registrationDate": refTime.AddDate(0, 0, -40)},
			expected: true,
		},
		{
			name: "registered 10 days ago is older_than 30 days -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldRegistrationDate,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"registrationDate": refTime.AddDate(0, 0, -10)},
			expected: false,
		},
		{
			name: "registered 5 days ago is newer_than 10 days -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRegistrationDate,
				Operator: domain.OperatorNewerThan,
				Value:    10,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"registrationDate": refTime.AddDate(0, 0, -5)},
			expected: true,
		},
		{
			name: "registrationDate before 2026-01-01 when registered in 2025 -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRegistrationDate,
				Operator: domain.OperatorBefore,
				Value:    "2026-01-01T00:00:00Z",
			},
			ctx:      map[string]any{"registrationDate": time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)},
			expected: true,
		},
		{
			name: "registrationDate after 2026-01-01 when registered in 2026-05 -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldRegistrationDate,
				Operator: domain.OperatorAfter,
				Value:    "2026-01-01",
			},
			ctx:      map[string]any{"registrationDate": "2026-05-10T10:00:00Z"},
			expected: true,
		},
		{
			name: "lastActivityAt 40 days ago is older_than 30 days -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"lastActivityAt": refTime.AddDate(0, 0, -40)},
			expected: true,
		},
		{
			name: "lastActivityAt exactly 30 days ago is older_than 30 days -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"lastActivityAt": refTime.AddDate(0, 0, -30)},
			expected: true,
		},
		{
			name: "lastActivityAt 29d 23h 59m 59s ago is older_than 30 days -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"lastActivityAt": refTime.AddDate(0, 0, -30).Add(time.Second)},
			expected: false,
		},
		{
			name: "lastActivityAt 31 days ago is older_than 30 days -> true",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"lastActivityAt": refTime.AddDate(0, 0, -31)},
			expected: true,
		},
		{
			name: "lastActivityAt null is older_than 30 days -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{"lastActivityAt": nil},
			expected: false,
		},
		{
			name: "lastActivityAt missing from context is older_than 30 days -> false",
			item: domain.ConditionItem{
				Field:    domain.FieldLastActivityAt,
				Operator: domain.OperatorOlderThan,
				Value:    30,
				Unit:     domain.UnitDays,
			},
			ctx:      map[string]any{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			group := domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &tc.item},
				},
			}
			matched, err := eval.Evaluate(group, tc.ctx, refTime)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, matched)
			}
		})
	}
}

func TestEvaluator_NestedConditionGroups(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Now().UTC()

	// Condition: (isBlocked == false) AND (role == "admin" OR role == "owner")
	group := domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldIsBlocked,
					Operator: domain.OperatorEquals,
					Value:    false,
				},
			},
			{
				Group: &domain.ConditionGroup{
					Operator: domain.GroupOperatorAny,
					Conditions: []domain.ConditionNode{
						{
							Item: &domain.ConditionItem{
								Field:    domain.FieldRole,
								Operator: domain.OperatorEquals,
								Value:    "admin",
							},
						},
						{
							Item: &domain.ConditionItem{
								Field:    domain.FieldRole,
								Operator: domain.OperatorEquals,
								Value:    "owner",
							},
						},
					},
				},
			},
		},
	}

	// 1. Matches: unblocked admin
	matched, err := eval.Evaluate(group, map[string]any{"isBlocked": false, "role": "admin"}, refTime)
	if err != nil || !matched {
		t.Errorf("expected matched=true, got %v, err=%v", matched, err)
	}

	// 2. Matches: unblocked owner
	matched, err = eval.Evaluate(group, map[string]any{"isBlocked": false, "role": "Owner"}, refTime)
	if err != nil || !matched {
		t.Errorf("expected matched=true, got %v, err=%v", matched, err)
	}

	// 3. Fails: blocked admin
	matched, err = eval.Evaluate(group, map[string]any{"isBlocked": true, "role": "admin"}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false, got %v, err=%v", matched, err)
	}

	// 4. Fails: unblocked regular user
	matched, err = eval.Evaluate(group, map[string]any{"isBlocked": false, "role": "user"}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false, got %v, err=%v", matched, err)
	}
}

func TestEvaluator_InactivityCombinedConditions(t *testing.T) {
	eval := NewEvaluator()
	refTime := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	// Condition: (role == "user") AND (isBlocked == false) AND (lastActivityAt older_than 30 days) AND (language == "en")
	group := domain.ConditionGroup{
		Operator: domain.GroupOperatorAll,
		Conditions: []domain.ConditionNode{
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldRole,
					Operator: domain.OperatorEquals,
					Value:    "user",
				},
			},
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldIsBlocked,
					Operator: domain.OperatorEquals,
					Value:    false,
				},
			},
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldLastActivityAt,
					Operator: domain.OperatorOlderThan,
					Value:    30,
					Unit:     domain.UnitDays,
				},
			},
			{
				Item: &domain.ConditionItem{
					Field:    domain.FieldLanguage,
					Operator: domain.OperatorEquals,
					Value:    "en",
				},
			},
		},
	}

	// 1. Matches: unblocked english user inactive for 45 days
	matched, err := eval.Evaluate(group, map[string]any{
		"role":           "User",
		"isBlocked":      false,
		"lastActivityAt": refTime.AddDate(0, 0, -45),
		"language":       "en",
	}, refTime)
	if err != nil || !matched {
		t.Errorf("expected matched=true, got %v, err=%v", matched, err)
	}

	// 2. Fails: user active 10 days ago
	matched, err = eval.Evaluate(group, map[string]any{
		"role":           "user",
		"isBlocked":      false,
		"lastActivityAt": refTime.AddDate(0, 0, -10),
		"language":       "en",
	}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false for active user, got %v, err=%v", matched, err)
	}

	// 3. Fails: user with null lastActivityAt
	matched, err = eval.Evaluate(group, map[string]any{
		"role":           "user",
		"isBlocked":      false,
		"lastActivityAt": nil,
		"language":       "en",
	}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false for null lastActivityAt, got %v, err=%v", matched, err)
	}

	// 4. Fails: inactive for 45 days, but blocked
	matched, err = eval.Evaluate(group, map[string]any{
		"role":           "user",
		"isBlocked":      true,
		"lastActivityAt": refTime.AddDate(0, 0, -45),
		"language":       "en",
	}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false for blocked user, got %v, err=%v", matched, err)
	}

	// 5. Fails: inactive for 45 days, but language is de
	matched, err = eval.Evaluate(group, map[string]any{
		"role":           "user",
		"isBlocked":      false,
		"lastActivityAt": refTime.AddDate(0, 0, -45),
		"language":       "de",
	}, refTime)
	if err != nil || matched {
		t.Errorf("expected matched=false for non-matching language, got %v, err=%v", matched, err)
	}
}
