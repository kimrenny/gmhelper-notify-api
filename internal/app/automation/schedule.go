package automation

import (
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

// IsDue determines if an automation rule is due for scheduled evaluation at reference time now.
func IsDue(rule *domain.AutomationRule, now time.Time) bool {
	if rule == nil || !rule.Enabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(rule.Config.Trigger), domain.TriggerUserInactive) {
		return false
	}
	now = now.UTC()

	switch rule.Config.Schedule.Type {
	case domain.ScheduleTypeDaily:
		return isDailyDue(rule, now)
	case domain.ScheduleTypeWeekly:
		return isWeeklyDue(rule, now)
	case domain.ScheduleTypeIntervalHours:
		return isIntervalDue(rule, now)
	default:
		return false
	}
}

// NextEvaluationTime calculates the next due timestamp for an automation rule.
func NextEvaluationTime(schedule domain.ScheduleConfig, lastEvaluatedAt *time.Time, createdAt time.Time, now time.Time) *time.Time {
	now = now.UTC()
	createdAt = createdAt.UTC()
	var lastEval *time.Time
	if lastEvaluatedAt != nil {
		t := lastEvaluatedAt.UTC()
		lastEval = &t
	}

	switch schedule.Type {
	case domain.ScheduleTypeDaily:
		return nextDailyEvaluation(schedule, lastEval, createdAt, now)
	case domain.ScheduleTypeWeekly:
		return nextWeeklyEvaluation(schedule, lastEval, createdAt, now)
	case domain.ScheduleTypeIntervalHours:
		return nextIntervalEvaluation(schedule, lastEval, createdAt, now)
	default:
		return nil
	}
}

func isDailyDue(rule *domain.AutomationRule, now time.Time) bool {
	hour := 0
	if rule.Config.Schedule.HourUTC != nil {
		hour = *rule.Config.Schedule.HourUTC
	}
	min := 0
	if rule.Config.Schedule.MinuteUTC != nil {
		min = *rule.Config.Schedule.MinuteUTC
	}

	todaySlot := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	if now.Before(todaySlot) {
		return false
	}

	if rule.LastEvaluatedAt != nil && !rule.LastEvaluatedAt.UTC().Before(todaySlot) {
		return false
	}

	if rule.CreatedAt.UTC().After(todaySlot) {
		return false
	}

	return true
}

func nextDailyEvaluation(schedule domain.ScheduleConfig, lastEvaluatedAt *time.Time, createdAt time.Time, now time.Time) *time.Time {
	hour := 0
	if schedule.HourUTC != nil {
		hour = *schedule.HourUTC
	}
	min := 0
	if schedule.MinuteUTC != nil {
		min = *schedule.MinuteUTC
	}

	todaySlot := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)

	// If today's slot is in the future relative to now
	if now.Before(todaySlot) {
		return &todaySlot
	}

	// If already evaluated for today's slot, or created after today's slot, or due now
	next := todaySlot.AddDate(0, 0, 1)
	return &next
}

func isWeeklyDue(rule *domain.AutomationRule, now time.Time) bool {
	dayOfWeek := 0
	if rule.Config.Schedule.DayOfWeek != nil {
		dayOfWeek = *rule.Config.Schedule.DayOfWeek
	}
	hour := 0
	if rule.Config.Schedule.HourUTC != nil {
		hour = *rule.Config.Schedule.HourUTC
	}
	min := 0
	if rule.Config.Schedule.MinuteUTC != nil {
		min = *rule.Config.Schedule.MinuteUTC
	}

	daysSince := (int(now.Weekday()) - dayOfWeek + 7) % 7
	mostRecentSlot := time.Date(now.Year(), now.Month(), now.Day()-daysSince, hour, min, 0, 0, time.UTC)

	if daysSince == 0 && now.Before(mostRecentSlot) {
		// Today is the scheduled day of the week, but before the scheduled hour:minute
		return false
	}

	if rule.LastEvaluatedAt != nil && !rule.LastEvaluatedAt.UTC().Before(mostRecentSlot) {
		return false
	}

	if rule.CreatedAt.UTC().After(mostRecentSlot) {
		return false
	}

	return true
}

func nextWeeklyEvaluation(schedule domain.ScheduleConfig, lastEvaluatedAt *time.Time, createdAt time.Time, now time.Time) *time.Time {
	dayOfWeek := 0
	if schedule.DayOfWeek != nil {
		dayOfWeek = *schedule.DayOfWeek
	}
	hour := 0
	if schedule.HourUTC != nil {
		hour = *schedule.HourUTC
	}
	min := 0
	if schedule.MinuteUTC != nil {
		min = *schedule.MinuteUTC
	}

	daysSince := (int(now.Weekday()) - dayOfWeek + 7) % 7
	mostRecentSlot := time.Date(now.Year(), now.Month(), now.Day()-daysSince, hour, min, 0, 0, time.UTC)

	if daysSince == 0 && now.Before(mostRecentSlot) {
		// Today is the scheduled day, and the time is still in the future
		return &mostRecentSlot
	}

	next := mostRecentSlot.AddDate(0, 0, 7)
	return &next
}

func isIntervalDue(rule *domain.AutomationRule, now time.Time) bool {
	if rule.Config.Schedule.IntervalHours == nil || *rule.Config.Schedule.IntervalHours < 1 {
		return false
	}
	interval := time.Duration(*rule.Config.Schedule.IntervalHours) * time.Hour

	base := rule.CreatedAt.UTC()
	if rule.LastEvaluatedAt != nil {
		base = rule.LastEvaluatedAt.UTC()
	}

	nextSlot := base.Add(interval)
	return !now.Before(nextSlot)
}

func nextIntervalEvaluation(schedule domain.ScheduleConfig, lastEvaluatedAt *time.Time, createdAt time.Time, now time.Time) *time.Time {
	if schedule.IntervalHours == nil || *schedule.IntervalHours < 1 {
		return nil
	}
	interval := time.Duration(*schedule.IntervalHours) * time.Hour

	if lastEvaluatedAt != nil {
		next := lastEvaluatedAt.Add(interval)
		if !now.Before(next) {
			next = now.Add(interval)
		}
		return &next
	}

	firstSlot := createdAt.Add(interval)
	if !now.Before(firstSlot) {
		next := now.Add(interval)
		return &next
	}
	return &firstSlot
}
