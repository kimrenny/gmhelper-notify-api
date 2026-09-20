package automation

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

// Evaluator evaluates automation condition ASTs against a unified user/event context dictionary.
type Evaluator struct{}

// NewEvaluator creates a new Evaluator instance.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate evaluates a ConditionGroup against the provided contextData using referenceTime for date calculations.
func (e *Evaluator) Evaluate(group domain.ConditionGroup, contextData map[string]any, referenceTime time.Time) (bool, error) {
	if referenceTime.IsZero() {
		referenceTime = time.Now().UTC()
	}
	return evaluateGroup(group, contextData, referenceTime)
}

func evaluateGroup(group domain.ConditionGroup, contextData map[string]any, referenceTime time.Time) (bool, error) {
	op := strings.ToLower(strings.TrimSpace(group.Operator))
	if op == "" {
		op = domain.GroupOperatorAll
	}

	if len(group.Conditions) == 0 {
		if op == domain.GroupOperatorAll {
			return true, nil
		}
		return false, nil
	}

	switch op {
	case domain.GroupOperatorAll:
		for _, node := range group.Conditions {
			matched, err := evaluateNode(node, contextData, referenceTime)
			if err != nil {
				return false, err
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil

	case domain.GroupOperatorAny:
		for _, node := range group.Conditions {
			matched, err := evaluateNode(node, contextData, referenceTime)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, fmt.Errorf("unsupported condition group operator: %q", group.Operator)
	}
}

func evaluateNode(node domain.ConditionNode, contextData map[string]any, referenceTime time.Time) (bool, error) {
	if node.Group != nil {
		return evaluateGroup(*node.Group, contextData, referenceTime)
	}
	if node.Item != nil {
		return evaluateItem(*node.Item, contextData, referenceTime)
	}
	return false, nil
}

func evaluateItem(item domain.ConditionItem, contextData map[string]any, referenceTime time.Time) (bool, error) {
	field := strings.TrimSpace(item.Field)
	operator := strings.ToLower(strings.TrimSpace(item.Operator))

	if field == "" || operator == "" {
		return false, nil
	}

	rawVal, exists := lookupField(contextData, field)

	switch field {
	case domain.FieldIsBlocked, domain.FieldIsActive:
		return evaluateBooleanField(operator, item.Value, rawVal, exists)

	case domain.FieldLanguage, domain.FieldRole:
		return evaluateEnumField(operator, item.Value, rawVal, exists)

	case domain.FieldEmail, domain.FieldUsername:
		return evaluateTextField(operator, item.Value, rawVal, exists)

	case domain.FieldRegistrationDate, domain.FieldLastActivityAt, domain.FieldLastActivity:
		return evaluateDateField(operator, item.Value, item.Unit, rawVal, exists, referenceTime)

	default:
		// Fallback for custom fields or arbitrary attributes: evaluate as text/scalar if possible
		return evaluateTextField(operator, item.Value, rawVal, exists)
	}
}

func lookupField(contextData map[string]any, field string) (any, bool) {
	if contextData == nil {
		return nil, false
	}

	// 1. Direct match
	if val, ok := contextData[field]; ok {
		return val, true
	}

	// 2. Case-insensitive lookup (e.g. "isBlocked" vs "isblocked" vs "IsBlocked")
	for k, v := range contextData {
		if strings.EqualFold(k, field) {
			return v, true
		}
	}

	return nil, false
}

func evaluateBooleanField(operator string, targetVal, actualVal any, exists bool) (bool, error) {
	if !exists || actualVal == nil {
		return false, nil
	}

	actualBool, ok := toBool(actualVal)
	if !ok {
		return false, nil
	}

	targetBool, ok := toBool(targetVal)
	if !ok {
		return false, nil
	}

	switch operator {
	case domain.OperatorEquals:
		return actualBool == targetBool, nil
	case domain.OperatorNotEquals:
		return actualBool != targetBool, nil
	default:
		return false, fmt.Errorf("unsupported boolean operator: %q", operator)
	}
}

func evaluateEnumField(operator string, targetVal, actualVal any, exists bool) (bool, error) {
	if !exists || actualVal == nil {
		if operator == domain.OperatorNotEquals || operator == domain.OperatorNotIn {
			return true, nil
		}
		return false, nil
	}

	actualStr := strings.TrimSpace(toString(actualVal))

	switch operator {
	case domain.OperatorEquals:
		targetStr := strings.TrimSpace(toString(targetVal))
		return strings.EqualFold(actualStr, targetStr), nil

	case domain.OperatorNotEquals:
		targetStr := strings.TrimSpace(toString(targetVal))
		return !strings.EqualFold(actualStr, targetStr), nil

	case domain.OperatorIn:
		targets := toStringSlice(targetVal)
		for _, t := range targets {
			if strings.EqualFold(actualStr, strings.TrimSpace(t)) {
				return true, nil
			}
		}
		return false, nil

	case domain.OperatorNotIn:
		targets := toStringSlice(targetVal)
		for _, t := range targets {
			if strings.EqualFold(actualStr, strings.TrimSpace(t)) {
				return false, nil
			}
		}
		return true, nil

	default:
		return false, fmt.Errorf("unsupported enum operator: %q", operator)
	}
}

func evaluateTextField(operator string, targetVal, actualVal any, exists bool) (bool, error) {
	switch operator {
	case domain.OperatorExists:
		if !exists || actualVal == nil {
			return false, nil
		}
		return strings.TrimSpace(toString(actualVal)) != "", nil

	case domain.OperatorNotExists:
		if !exists || actualVal == nil {
			return true, nil
		}
		return strings.TrimSpace(toString(actualVal)) == "", nil
	}

	if !exists || actualVal == nil {
		if operator == domain.OperatorNotEquals {
			return true, nil
		}
		return false, nil
	}

	actualStr := toString(actualVal)
	targetStr := toString(targetVal)

	switch operator {
	case domain.OperatorEquals:
		return strings.EqualFold(strings.TrimSpace(actualStr), strings.TrimSpace(targetStr)), nil

	case domain.OperatorNotEquals:
		return !strings.EqualFold(strings.TrimSpace(actualStr), strings.TrimSpace(targetStr)), nil

	case domain.OperatorContains:
		return strings.Contains(strings.ToLower(actualStr), strings.ToLower(targetStr)), nil

	case domain.OperatorStartsWith:
		return strings.HasPrefix(strings.ToLower(actualStr), strings.ToLower(targetStr)), nil

	case domain.OperatorEndsWith:
		return strings.HasSuffix(strings.ToLower(actualStr), strings.ToLower(targetStr)), nil

	default:
		return false, fmt.Errorf("unsupported text operator: %q", operator)
	}
}

func evaluateDateField(operator string, targetVal any, unit string, actualVal any, exists bool, referenceTime time.Time) (bool, error) {
	if !exists || actualVal == nil {
		return false, nil
	}

	actualTime, ok := toTime(actualVal)
	if !ok || actualTime.IsZero() {
		return false, nil
	}

	switch operator {
	case domain.OperatorOlderThan, domain.OperatorNewerThan:
		durationNum, ok := toFloat64(targetVal)
		if !ok || durationNum <= 0 {
			return false, nil
		}

		u := strings.ToLower(strings.TrimSpace(unit))
		intDuration := int(durationNum)

		var thresholdTime time.Time
		switch u {
		case domain.UnitDays, "day", "d":
			thresholdTime = referenceTime.AddDate(0, 0, -intDuration)
		case domain.UnitMonths, "month", "m":
			thresholdTime = referenceTime.AddDate(0, -intDuration, 0)
		case domain.UnitYears, "year", "y":
			thresholdTime = referenceTime.AddDate(-intDuration, 0, 0)
		default:
			// Default to days
			thresholdTime = referenceTime.AddDate(0, 0, -intDuration)
		}

		if operator == domain.OperatorOlderThan {
			// Older than N days/months/years means actual time is at or before the threshold
			return !actualTime.After(thresholdTime), nil
		}
		// Newer than N days/months/years means actual registration occurred AFTER the threshold
		return actualTime.After(thresholdTime), nil

	case domain.OperatorBefore, domain.OperatorAfter:
		targetTime, ok := toTime(targetVal)
		if !ok || targetTime.IsZero() {
			return false, nil
		}

		if operator == domain.OperatorBefore {
			return actualTime.Before(targetTime), nil
		}
		return actualTime.After(targetTime), nil

	default:
		return false, fmt.Errorf("unsupported date operator: %q", operator)
	}
}

// Helpers for scalar conversions

func toBool(v any) (bool, bool) {
	if v == nil {
		return false, false
	}
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		return b, err == nil
	default:
		return false, false
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		res := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				res = append(res, s)
			} else {
				res = append(res, fmt.Sprintf("%v", item))
			}
		}
		return res
	case string:
		parts := strings.Split(val, ",")
		res := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				res = append(res, trimmed)
			}
		}
		return res
	default:
		return []string{fmt.Sprintf("%v", val)}
	}
}

func toFloat64(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toTime(v any) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	switch val := v.(type) {
	case time.Time:
		return val.UTC(), true
	case *time.Time:
		if val == nil {
			return time.Time{}, false
		}
		return val.UTC(), true
	case string:
		s := strings.TrimSpace(val)
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC(), true
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC(), true
		}
		if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
			return t.UTC(), true
		}
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UTC(), true
		}
		return time.Time{}, false
	default:
		return time.Time{}, false
	}
}
