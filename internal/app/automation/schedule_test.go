package automation

import (
	"context"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestIsDue_Daily(t *testing.T) {
	createdAt := time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC)
	rule := &domain.AutomationRule{
		ID:        "rule-daily-1",
		Enabled:   true,
		CreatedAt: createdAt,
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
		},
	}

	// 1. Before scheduled time (08:59:59) -> not due
	beforeTime := time.Date(2026, 9, 20, 8, 59, 59, 0, time.UTC)
	if IsDue(rule, beforeTime) {
		t.Errorf("expected daily rule to NOT be due before scheduled time")
	}
	nextBefore := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, beforeTime)
	expectedToday := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	if nextBefore == nil || !nextBefore.Equal(expectedToday) {
		t.Errorf("expected next evaluation to be %v, got %v", expectedToday, nextBefore)
	}

	// 2. Exactly at scheduled time (09:00:00) -> due
	exactTime := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	if !IsDue(rule, exactTime) {
		t.Errorf("expected daily rule to BE due exactly at scheduled time")
	}

	// 3. After scheduled time (09:05:00, delayed tick, not evaluated yet) -> due
	afterTime := time.Date(2026, 9, 20, 9, 5, 0, 0, time.UTC)
	if !IsDue(rule, afterTime) {
		t.Errorf("expected daily rule to BE due on delayed tick after scheduled time")
	}

	// 4. Same day must not execute twice: after evaluating at 09:05:00 -> not due at 09:30:00
	rule.LastEvaluatedAt = timePtr(afterTime)
	sameDayLater := time.Date(2026, 9, 20, 9, 30, 0, 0, time.UTC)
	if IsDue(rule, sameDayLater) {
		t.Errorf("expected daily rule to NOT be due twice on the same day")
	}
	nextLater := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, sameDayLater)
	expectedTomorrow := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if nextLater == nil || !nextLater.Equal(expectedTomorrow) {
		t.Errorf("expected next evaluation to be tomorrow %v, got %v", expectedTomorrow, nextLater)
	}

	// 5. Next day (2026-09-21 09:00:00) -> due
	nextDayTime := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if !IsDue(rule, nextDayTime) {
		t.Errorf("expected daily rule to BE due on the next day at scheduled time")
	}

	// 6. Rule created after today's scheduled time (created at 10:00, scheduled at 09:00) -> not due on creation day
	lateRule := &domain.AutomationRule{
		ID:        "rule-daily-late",
		Enabled:   true,
		CreatedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
		},
	}
	at1030 := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	if IsDue(lateRule, at1030) {
		t.Errorf("expected rule created after today's schedule to NOT be due on creation day")
	}
	nextLate := NextEvaluationTime(lateRule.Config.Schedule, lateRule.LastEvaluatedAt, lateRule.CreatedAt, at1030)
	if nextLate == nil || !nextLate.Equal(expectedTomorrow) {
		t.Errorf("expected next evaluation for late created rule to be %v, got %v", expectedTomorrow, nextLate)
	}
}

func TestIsDue_Weekly(t *testing.T) {
	// Rule created on Monday 2026-09-14 06:00:00 UTC, scheduled Monday (1) at 09:00 UTC
	createdAt := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC)
	rule := &domain.AutomationRule{
		ID:        "rule-weekly-1",
		Enabled:   true,
		CreatedAt: createdAt,
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeWeekly,
				DayOfWeek: intPtr(1), // Monday
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
		},
	}

	// 1. Correct weekday before scheduled time: Monday 2026-09-21 08:30:00 -> not due
	mondayBefore := time.Date(2026, 9, 21, 8, 30, 0, 0, time.UTC)
	// (Simulate that rule ran previous week on 2026-09-14 09:00)
	rule.LastEvaluatedAt = timePtr(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	if IsDue(rule, mondayBefore) {
		t.Errorf("expected weekly rule to NOT be due before scheduled time on Monday")
	}
	nextBefore := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, mondayBefore)
	expectedMonday := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if nextBefore == nil || !nextBefore.Equal(expectedMonday) {
		t.Errorf("expected next evaluation to be Monday %v, got %v", expectedMonday, nextBefore)
	}

	// 2. Exactly at scheduled time: Monday 2026-09-21 09:00:00 -> due
	mondayExact := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if !IsDue(rule, mondayExact) {
		t.Errorf("expected weekly rule to BE due exactly at Monday 09:00")
	}

	// 3. After scheduled time on same day (delayed tick): Monday 2026-09-21 14:00:00 -> due if not yet evaluated
	mondayAfter := time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC)
	if !IsDue(rule, mondayAfter) {
		t.Errorf("expected weekly rule to BE due on delayed tick on Monday afternoon")
	}

	// 4. Same weekly occurrence cannot execute twice: evaluated at Monday 14:00:00 -> not due later Monday
	rule.LastEvaluatedAt = timePtr(mondayAfter)
	mondayLater := time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
	if IsDue(rule, mondayLater) {
		t.Errorf("expected weekly rule to NOT be due again on the same Monday")
	}

	// 5. Missed occurrence catch-up: service down all Monday, starts on Tuesday 2026-09-22 10:00:00 -> missed Monday occurrence IS due
	ruleMissedMonday := &domain.AutomationRule{
		ID:              "rule-weekly-missed",
		Enabled:         true,
		CreatedAt:       createdAt,
		LastEvaluatedAt: timePtr(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)), // ran last week, missed Monday 2026-09-21
		Config:          rule.Config,
	}
	tuesdayCatchUp := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	if !IsDue(ruleMissedMonday, tuesdayCatchUp) {
		t.Errorf("expected missed Monday weekly occurrence to BE due on Tuesday catch-up")
	}
	nextCatchUp := NextEvaluationTime(ruleMissedMonday.Config.Schedule, ruleMissedMonday.LastEvaluatedAt, ruleMissedMonday.CreatedAt, tuesdayCatchUp)
	expectedNextMonday := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	if nextCatchUp == nil || !nextCatchUp.Equal(expectedNextMonday) {
		t.Errorf("expected next evaluation after catch-up to be next Monday %v, got %v", expectedNextMonday, nextCatchUp)
	}

	// 6. After catch-up executed on Tuesday -> not due on Wednesday, Thursday, Friday, Saturday, Sunday
	ruleMissedMonday.LastEvaluatedAt = timePtr(tuesdayCatchUp)
	wednesdayTime := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	if IsDue(ruleMissedMonday, wednesdayTime) {
		t.Errorf("expected weekly rule to NOT be due on Wednesday after Tuesday catch-up")
	}
	thursdayTime := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	if IsDue(ruleMissedMonday, thursdayTime) {
		t.Errorf("expected weekly rule to NOT be due on Thursday after Tuesday catch-up")
	}
	sundayTime := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)
	if IsDue(ruleMissedMonday, sundayTime) {
		t.Errorf("expected weekly rule to NOT be due on Sunday after Tuesday catch-up")
	}

	// 7. Next weekly occurrence: Monday 2026-09-28 09:00:00 -> due
	if !IsDue(ruleMissedMonday, expectedNextMonday) {
		t.Errorf("expected weekly rule to BE due on next Monday 09:00")
	}

	// 8. Rule created on Tuesday (created at Tuesday 2026-09-22 10:00) -> must NOT execute for past Monday
	ruleCreatedTuesday := &domain.AutomationRule{
		ID:        "rule-weekly-tue",
		Enabled:   true,
		CreatedAt: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
		Config:    rule.Config,
	}
	atTue1030 := time.Date(2026, 9, 22, 10, 30, 0, 0, time.UTC)
	if IsDue(ruleCreatedTuesday, atTue1030) {
		t.Errorf("expected rule created on Tuesday to NOT be due for past Monday")
	}
	nextTueCreated := NextEvaluationTime(ruleCreatedTuesday.Config.Schedule, nil, ruleCreatedTuesday.CreatedAt, atTue1030)
	if nextTueCreated == nil || !nextTueCreated.Equal(expectedNextMonday) {
		t.Errorf("expected next evaluation for Tuesday-created rule to be next Monday %v, got %v", expectedNextMonday, nextTueCreated)
	}
}

func TestIsDue_Daily_MissedOccurrences(t *testing.T) {
	// Rule created on 2026-09-15, scheduled daily at 09:00 UTC
	createdAt := time.Date(2026, 9, 15, 6, 0, 0, 0, time.UTC)
	rule := &domain.AutomationRule{
		ID:              "rule-daily-missed",
		Enabled:         true,
		CreatedAt:       createdAt,
		LastEvaluatedAt: timePtr(time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)),
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
		},
	}

	// 1. Service down for 3 days, restarts on 2026-09-18 at 14:00 UTC -> due once for today's missed slot
	restartDay := time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC)
	if !IsDue(rule, restartDay) {
		t.Errorf("expected daily rule to BE due on service restart after multiple down days")
	}
	nextRestart := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, restartDay)
	expectedTomorrow := time.Date(2026, 9, 19, 9, 0, 0, 0, time.UTC)
	if nextRestart == nil || !nextRestart.Equal(expectedTomorrow) {
		t.Errorf("expected next evaluation after restart to be tomorrow %v, got %v", expectedTomorrow, nextRestart)
	}

	// 2. Evaluated on 2026-09-18 14:00 -> subsequent tick at 14:30 is NOT due (no repeated replay of missed days)
	rule.LastEvaluatedAt = timePtr(restartDay)
	at1430 := time.Date(2026, 9, 18, 14, 30, 0, 0, time.UTC)
	if IsDue(rule, at1430) {
		t.Errorf("expected daily rule to NOT replay missed days on subsequent ticks")
	}
}

func TestIsDue_Interval(t *testing.T) {
	createdAt := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	rule := &domain.AutomationRule{
		ID:        "rule-interval-1",
		Enabled:   true,
		CreatedAt: createdAt,
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:          domain.ScheduleTypeIntervalHours,
				IntervalHours: intPtr(4),
			},
		},
	}

	// 1. Before interval: at 12:00:00 (2 hours after creation) -> not due
	at12 := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	if IsDue(rule, at12) {
		t.Errorf("expected interval rule to NOT be due before interval expires")
	}
	next12 := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, at12)
	expectedFirst := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)
	if next12 == nil || !next12.Equal(expectedFirst) {
		t.Errorf("expected next evaluation to be %v, got %v", expectedFirst, next12)
	}

	// 2. Exactly at interval: at 14:00:00 -> due
	at14 := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)
	if !IsDue(rule, at14) {
		t.Errorf("expected interval rule to BE due exactly at interval")
	}

	// 3. After interval (delayed tick): at 14:10:00 -> due
	at1410 := time.Date(2026, 9, 20, 14, 10, 0, 0, time.UTC)
	if !IsDue(rule, at1410) {
		t.Errorf("expected interval rule to BE due after interval expiration")
	}

	// 4. Repeated scheduler tick: evaluated at 14:10:00 -> tick at 14:15:00 is not due
	rule.LastEvaluatedAt = timePtr(at1410)
	at1415 := time.Date(2026, 9, 20, 14, 15, 0, 0, time.UTC)
	if IsDue(rule, at1415) {
		t.Errorf("expected interval rule to NOT be due immediately after evaluation")
	}
	next1415 := NextEvaluationTime(rule.Config.Schedule, rule.LastEvaluatedAt, rule.CreatedAt, at1415)
	expectedSecond := time.Date(2026, 9, 20, 18, 10, 0, 0, time.UTC)
	if next1415 == nil || !next1415.Equal(expectedSecond) {
		t.Errorf("expected next evaluation to be %v, got %v", expectedSecond, next1415)
	}

	// 5. Service restart after outage: service restarts at 2026-09-22 10:00 (2 days later)
	atRestart := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	if !IsDue(rule, atRestart) {
		t.Errorf("expected overdue interval rule to BE due on service restart")
	}
}

func TestIsDue_Boundaries(t *testing.T) {
	// UTC Midnight transition
	// Rule scheduled daily at 00:00 UTC
	midnightRule := &domain.AutomationRule{
		ID:        "rule-midnight",
		Enabled:   true,
		CreatedAt: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(0),
				MinuteUTC: intPtr(0),
			},
		},
		LastEvaluatedAt: timePtr(time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)),
	}

	// Just before midnight: 2026-09-19 23:59:59.999 UTC -> not due (already evaluated for 2026-09-19)
	beforeMidnight := time.Date(2026, 9, 19, 23, 59, 59, 999000000, time.UTC)
	if IsDue(midnightRule, beforeMidnight) {
		t.Errorf("expected midnight rule to NOT be due before midnight")
	}

	// Exactly at midnight: 2026-09-20 00:00:00 UTC -> due
	atMidnight := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	if !IsDue(midnightRule, atMidnight) {
		t.Errorf("expected midnight rule to BE due at 00:00:00 UTC")
	}

	// Just after midnight: 2026-09-20 00:00:01 UTC -> due if not yet evaluated
	afterMidnight := time.Date(2026, 9, 20, 0, 0, 1, 0, time.UTC)
	if !IsDue(midnightRule, afterMidnight) {
		t.Errorf("expected midnight rule to BE due at 00:00:01 UTC before evaluation")
	}
}

func TestIsDue_NonScheduledAndDisabled(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	// Event-driven rule must NEVER be due in scheduler
	eventRule := &domain.AutomationRule{
		ID:        "rule-event",
		Enabled:   true,
		CreatedAt: now.Add(-10 * time.Hour),
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserRegistered,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(12),
				MinuteUTC: intPtr(0),
			},
		},
	}
	if IsDue(eventRule, now) {
		t.Errorf("event-driven rule must never be due in background scheduler")
	}
	if next := NextEvaluationTime(eventRule.Config.Schedule, nil, eventRule.CreatedAt, now); next == nil {
		// next calculation for non-inactivity or schedule
	}

	// Disabled inactivity rule must NEVER be due
	disabledRule := &domain.AutomationRule{
		ID:        "rule-disabled",
		Enabled:   false,
		CreatedAt: now.Add(-10 * time.Hour),
		Config: domain.AutomationRuleConfig{
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(12),
				MinuteUTC: intPtr(0),
			},
		},
	}
	if IsDue(disabledRule, now) {
		t.Errorf("disabled rule must never be due in background scheduler")
	}
}

func TestService_NextEvaluationAt_Lifecycle(t *testing.T) {
	ruleRepo := &mockRuleRepo{rules: []*domain.AutomationRule{}}
	templateRepo := &engineMockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-1": {
				ID:           "tpl-1",
				Status:       domain.TemplateStatusActive,
				TemplateType: domain.TemplateTypeAutomation,
				Subject:      "Subject",
				HTMLBody:     "<p>Body</p>",
			},
		},
	}
	svc := NewService(ruleRepo, templateRepo, nil, nil)
	ctx := context.Background()

	// 1. Create user.inactive rule -> NextEvaluationAt is populated
	inactRule, err := svc.Create(ctx, CreateInput{
		Name:       "Inactivity Alert",
		TemplateID: "tpl-1",
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
			Conditions: domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
				},
			},
			Action: domain.ActionConfig{Type: domain.ActionTypeSendEmail},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating inactivity rule: %v", err)
	}
	if inactRule.NextEvaluationAt == nil {
		t.Fatalf("expected NextEvaluationAt to be populated for enabled inactivity rule")
	}

	// 2. Create user.registered rule -> NextEvaluationAt is nil
	eventRule, err := svc.Create(ctx, CreateInput{
		Name:       "Registration Alert",
		TemplateID: "tpl-1",
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserRegistered,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
			Conditions: domain.ConditionGroup{
				Operator: domain.GroupOperatorAll,
				Conditions: []domain.ConditionNode{
					{Item: &domain.ConditionItem{Field: domain.FieldIsActive, Operator: domain.OperatorEquals, Value: true}},
				},
			},
			Action: domain.ActionConfig{Type: domain.ActionTypeSendEmail},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating event rule: %v", err)
	}
	if eventRule.NextEvaluationAt != nil {
		t.Errorf("expected NextEvaluationAt to be nil for event-driven rule, got %v", eventRule.NextEvaluationAt)
	}

	// 3. Disable inactivity rule -> NextEvaluationAt cleared to nil
	disabled, err := svc.Disable(ctx, inactRule.ID)
	if err != nil {
		t.Fatalf("unexpected error disabling rule: %v", err)
	}
	if disabled.NextEvaluationAt != nil {
		t.Errorf("expected NextEvaluationAt to be nil when rule is disabled, got %v", disabled.NextEvaluationAt)
	}

	// 4. Re-enable rule -> NextEvaluationAt recalculated
	enabled, err := svc.Enable(ctx, inactRule.ID)
	if err != nil {
		t.Fatalf("unexpected error enabling rule: %v", err)
	}
	if enabled.NextEvaluationAt == nil {
		t.Errorf("expected NextEvaluationAt to be recalculated when rule is re-enabled")
	}

	// 5. Update schedule to weekly -> NextEvaluationAt recalculated according to weekly schedule
	updatedCfg := enabled.Config
	updatedCfg.Schedule = domain.ScheduleConfig{
		Type:      domain.ScheduleTypeWeekly,
		DayOfWeek: intPtr(1), // Monday
		HourUTC:   intPtr(15),
		MinuteUTC: intPtr(30),
	}
	updated, err := svc.Update(ctx, inactRule.ID, UpdateInput{
		Config: &updatedCfg,
	})
	if err != nil {
		t.Fatalf("unexpected error updating rule schedule: %v", err)
	}
	if updated.NextEvaluationAt == nil {
		t.Fatalf("expected NextEvaluationAt to be recalculated on schedule update")
	}
	if updated.NextEvaluationAt.Hour() != 15 || updated.NextEvaluationAt.Minute() != 30 {
		t.Errorf("expected next evaluation hour:minute to be 15:30, got %02d:%02d",
			updated.NextEvaluationAt.Hour(), updated.NextEvaluationAt.Minute())
	}
}

func TestIsDue_RespectsFutureNextEvaluationAt(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)
	futureNext := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)

	rule := &domain.AutomationRule{
		ID:      "rule-future-test",
		Enabled: true,
		Config: domain.AutomationRuleConfig{
			Version: 1,
			Trigger: domain.TriggerUserInactive,
			Schedule: domain.ScheduleConfig{
				Type:      domain.ScheduleTypeDaily,
				HourUTC:   intPtr(9),
				MinuteUTC: intPtr(0),
			},
		},
		CreatedAt:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		LastEvaluatedAt:  nil,
		NextEvaluationAt: &futureNext,
	}

	// At 14:00 today (before futureNext tomorrow at 09:00), IsDue must return false
	if IsDue(rule, now) {
		t.Errorf("expected IsDue to return false when now is before NextEvaluationAt")
	}

	// When now reaches futureNext, IsDue returns true
	if !IsDue(rule, futureNext) {
		t.Errorf("expected IsDue to return true when now reaches NextEvaluationAt")
	}
}
