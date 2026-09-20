package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
	"github.com/google/uuid"
)

// UserResolver defines user lookup capabilities needed by the automation engine.
type UserResolver interface {
	GetUserByID(ctx context.Context, id string) (*userclient.User, error)
}

// Engine evaluates automation rules against runtime events and dispatches notifications.
type Engine struct {
	ruleRepo     domain.AutomationRuleRepository
	templateRepo domain.EmailTemplateRepository
	directRepo   domain.DirectNotificationRepository
	execRepo     domain.AutomationExecutionRepository
	userResolver UserResolver
	evaluator    *Evaluator
	audit        *audit.Service
	logger       logger.Logger
}

// NewEngine constructs a new Automation Runtime Engine.
func NewEngine(
	ruleRepo domain.AutomationRuleRepository,
	templateRepo domain.EmailTemplateRepository,
	directRepo domain.DirectNotificationRepository,
	execRepo domain.AutomationExecutionRepository,
	userResolver UserResolver,
	auditSvc *audit.Service,
	log logger.Logger,
) *Engine {
	if log == nil {
		log = logger.NewNop()
	}
	return &Engine{
		ruleRepo:     ruleRepo,
		templateRepo: templateRepo,
		directRepo:   directRepo,
		execRepo:     execRepo,
		userResolver: userResolver,
		evaluator:    NewEvaluator(),
		audit:        auditSvc,
		logger:       log,
	}
}

// HandleEvent evaluates all enabled automation rules against the provided runtime event.
func (e *Engine) HandleEvent(ctx context.Context, event Event) (*EventExecutionResult, error) {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	result := &EventExecutionResult{
		EventID:   event.ID,
		EventType: event.Type,
		Results:   make([]RuleExecutionResult, 0),
	}

	if e.ruleRepo == nil {
		return result, errors.New("automation rule repository is nil")
	}

	// 1. Resolve user profile if not directly attached
	user := event.User
	if user == nil && strings.TrimSpace(event.UserID) != "" && e.userResolver != nil {
		resolved, err := e.userResolver.GetUserByID(ctx, event.UserID)
		if err != nil {
			e.logger.Warn("failed to resolve user for automation event",
				logger.String("eventId", event.ID),
				logger.String("userId", event.UserID),
				logger.Error(err),
			)
		} else {
			user = resolved
		}
	}

	// 2. Build unified context dictionary for condition evaluation and template rendering
	contextData := buildContextData(event, user)

	// 3. Load all enabled automation rules
	rules, err := e.ruleRepo.ListEnabled(ctx)
	if err != nil {
		e.logger.Error("failed to list enabled automation rules",
			logger.String("eventId", event.ID),
			logger.Error(err),
		)
		return result, fmt.Errorf("failed to list enabled automation rules: %w", err)
	}

	// 4. Evaluate each matching rule independently (Failure Isolation)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.Config.Trigger), strings.TrimSpace(event.Type)) {
			continue
		}
		ruleRes := e.executeRule(ctx, rule, event, user, contextData)
		result.Results = append(result.Results, ruleRes)
	}

	return result, nil
}

// UserLister defines the client contract for retrieving paginated users.
type UserLister interface {
	GetUsers(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error)
}

// InactivityEvaluationSummary summarizes the outcome of an inactivity evaluation run across all users.
type InactivityEvaluationSummary struct {
	TotalUsersEvaluated    int `json:"totalUsersEvaluated"`
	TotalRulesEvaluated    int `json:"totalRulesEvaluated"`
	ExecutedCount          int `json:"executedCount"`
	SkippedNotMatchedCount int `json:"skippedNotMatchedCount"`
	SkippedCooldownCount   int `json:"skippedCooldownCount"`
	SkippedDuplicateCount  int `json:"skippedDuplicateCount"`
	FailedCount            int `json:"failedCount"`
}

// EvaluateInactivity evaluates enabled automation rules against all users fetched page-by-page.
func (e *Engine) EvaluateInactivity(ctx context.Context, userLister UserLister, now time.Time, pageSize int) (*InactivityEvaluationSummary, error) {
	if userLister == nil {
		return nil, errors.New("user lister is nil")
	}
	if e.ruleRepo == nil {
		return nil, errors.New("automation rule repository is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if pageSize <= 0 {
		pageSize = 250
	}

	rules, err := e.ruleRepo.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled automation rules: %w", err)
	}

	dueRules := make([]*domain.AutomationRule, 0, len(rules))
	for _, rule := range rules {
		if rule != nil && strings.EqualFold(strings.TrimSpace(rule.Config.Trigger), domain.TriggerUserInactive) {
			if IsDue(rule, now) {
				dueRules = append(dueRules, rule)
			}
		}
	}
	if len(dueRules) == 0 {
		return &InactivityEvaluationSummary{}, nil
	}

	summary := &InactivityEvaluationSummary{
		TotalRulesEvaluated: len(dueRules),
	}

	page := 1
	for {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}

		pagedUsers, err := userLister.GetUsers(ctx, page, pageSize, false, false)
		if err != nil {
			e.logger.Error("failed to retrieve users page for inactivity evaluation",
				logger.Int("page", page),
				logger.Error(err),
			)
			return summary, fmt.Errorf("failed to retrieve users page %d: %w", page, err)
		}

		if pagedUsers == nil || len(pagedUsers.Items) == 0 {
			break
		}

		for _, user := range pagedUsers.Items {
			summary.TotalUsersEvaluated++
			userCopy := user

			for _, rule := range dueRules {
				var eventID string
				if rule.Config.Action.CooldownDays != nil && *rule.Config.Action.CooldownDays > 0 {
					eventID = uuid.NewString()
				} else {
					eventID = fmt.Sprintf("inactivity:%s:%s", rule.ID, userCopy.ID)
				}

				event := Event{
					ID:         eventID,
					Type:       EventUserInactive,
					UserID:     userCopy.ID,
					User:       &userCopy,
					OccurredAt: now,
				}

				contextData := buildContextData(event, &userCopy)
				res := e.executeRule(ctx, rule, event, &userCopy, contextData)

				switch res.Status {
				case StatusExecuted:
					summary.ExecutedCount++
				case StatusSkippedNotMatched:
					summary.SkippedNotMatchedCount++
				case StatusSkippedCooldown:
					summary.SkippedCooldownCount++
				case StatusSkippedDuplicate:
					summary.SkippedDuplicateCount++
				case StatusFailed:
					summary.FailedCount++
				}
			}
		}

		if !pagedUsers.HasNextPage || len(pagedUsers.Items) < pageSize {
			break
		}
		page++
	}

	for _, rule := range dueRules {
		nextEval := NextEvaluationTime(rule.Config.Schedule, &now, rule.CreatedAt, now)
		rule.LastEvaluatedAt = &now
		rule.NextEvaluationAt = nextEval
		if err := e.ruleRepo.UpdateEvaluationTimes(ctx, rule.ID, &now, nextEval); err != nil {
			e.logger.Warn("failed to update evaluation timestamps for rule",
				logger.String("ruleId", rule.ID),
				logger.Error(err),
			)
		}
	}

	return summary, nil
}

// EvaluateUserInactivity evaluates all enabled automation rules against a single user at the given time.
func (e *Engine) EvaluateUserInactivity(ctx context.Context, user *userclient.User, now time.Time) ([]RuleExecutionResult, error) {
	if user == nil {
		return nil, errors.New("user cannot be nil")
	}
	if e.ruleRepo == nil {
		return nil, errors.New("automation rule repository is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	rules, err := e.ruleRepo.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled automation rules: %w", err)
	}

	inactivityRules := make([]*domain.AutomationRule, 0, len(rules))
	for _, rule := range rules {
		if rule != nil && strings.EqualFold(strings.TrimSpace(rule.Config.Trigger), domain.TriggerUserInactive) {
			inactivityRules = append(inactivityRules, rule)
		}
	}

	results := make([]RuleExecutionResult, 0, len(inactivityRules))
	for _, rule := range inactivityRules {
		var eventID string
		if rule.Config.Action.CooldownDays != nil && *rule.Config.Action.CooldownDays > 0 {
			eventID = uuid.NewString()
		} else {
			eventID = fmt.Sprintf("inactivity:%s:%s", rule.ID, user.ID)
		}

		event := Event{
			ID:         eventID,
			Type:       EventUserInactive,
			UserID:     user.ID,
			User:       user,
			OccurredAt: now,
		}

		contextData := buildContextData(event, user)
		res := e.executeRule(ctx, rule, event, user, contextData)
		results = append(results, res)
	}

	return results, nil
}

// EvaluateRule evaluates a single automation rule's conditions against a user/event context dictionary.
func (e *Engine) EvaluateRule(
	ctx context.Context,
	rule *domain.AutomationRule,
	contextData map[string]any,
	referenceTime time.Time,
) (bool, error) {
	if rule == nil || !rule.Enabled {
		return false, nil
	}
	return e.evaluator.Evaluate(rule.Config.Conditions, contextData, referenceTime)
}

func (e *Engine) executeRule(
	ctx context.Context,
	rule *domain.AutomationRule,
	event Event,
	user *userclient.User,
	contextData map[string]any,
) RuleExecutionResult {
	res := RuleExecutionResult{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		Status:   StatusSkippedNotMatched,
	}

	// 1. Condition evaluation
	matched, err := e.evaluator.Evaluate(rule.Config.Conditions, contextData, event.OccurredAt)
	if err != nil {
		errMsg := fmt.Sprintf("condition evaluation error: %v", err)
		e.logger.Warn("automation rule condition evaluation failed",
			logger.String("ruleId", rule.ID),
			logger.String("eventId", event.ID),
			logger.Error(err),
		)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	if !matched {
		res.Status = StatusSkippedNotMatched
		return res
	}

	// 2. Determine recipient email and user ID
	recipientEmail := extractRecipientEmail(user, event, contextData)
	if recipientEmail == "" || !isValidEmail(recipientEmail) {
		errMsg := fmt.Sprintf("invalid or missing recipient email address: %q", recipientEmail)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	var externalUserID *string
	if user != nil && user.ID != "" {
		externalUserID = &user.ID
	} else if strings.TrimSpace(event.UserID) != "" {
		uid := strings.TrimSpace(event.UserID)
		externalUserID = &uid
	}

	// 3. Template resolution & compatibility
	if e.templateRepo == nil {
		errMsg := "template repository is nil"
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	tpl, err := e.templateRepo.GetByID(ctx, rule.TemplateID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to resolve template %q: %v", rule.TemplateID, err)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}
	if tpl == nil {
		errMsg := fmt.Sprintf("template %q not found", rule.TemplateID)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	if tpl.Status != domain.TemplateStatusActive {
		errMsg := fmt.Sprintf("template %q is not active (status: %s)", tpl.ID, tpl.Status)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	if tpl.TemplateType != domain.TemplateTypeAutomation && tpl.TemplateType != domain.TemplateTypeDirect {
		errMsg := fmt.Sprintf("template %q has incompatible type %q for automation (must be automation or direct)", tpl.ID, tpl.TemplateType)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	// 4. Template rendering (strict variable enforcement via direct.RenderEmail)
	rendered, err := direct.RenderEmail(tpl.Subject, tpl.HTMLBody, tpl.PlainTextBody, contextData)
	if err != nil {
		errMsg := fmt.Sprintf("template rendering failed: %v", err)
		res.Status = StatusFailed
		res.Error = errMsg
		e.recordAuditFailure(ctx, rule, event, errMsg)
		return res
	}

	// 5. Prepare DirectNotification and AutomationExecution records
	recipientName := ""
	if user != nil {
		recipientName = user.Username
	}

	var payloadBytes json.RawMessage
	if b, err := json.Marshal(contextData); err == nil {
		payloadBytes = b
	}

	now := time.Now().UTC()
	notificationID := uuid.NewString()
	extUID := ""
	if externalUserID != nil {
		extUID = *externalUserID
	}
	notif := &domain.DirectNotification{
		ID:               notificationID,
		TemplateID:       tpl.ID,
		ExternalUserID:   extUID,
		RecipientEmail:   recipientEmail,
		RecipientName:    recipientName,
		NotificationType: domain.NotificationTypeAutomation,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    0,
		Payload:          payloadBytes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	exec := &domain.AutomationExecution{
		ID:             uuid.NewString(),
		RuleID:         rule.ID,
		EventID:        event.ID,
		RecipientEmail: recipientEmail,
		ExternalUserID: externalUserID,
		NotificationID: &notif.ID,
		Status:         "success",
		ExecutedAt:     event.OccurredAt,
		CreatedAt:      now,
	}

	// 6. Atomic execution: Idempotency check, Cooldown check, Notification creation, Execution record
	if e.execRepo != nil {
		execStatus, atomicErr := e.execRepo.ExecuteRuleAtomic(
			ctx,
			rule.ID,
			event.ID,
			recipientEmail,
			externalUserID,
			rule.Config.Action.CooldownDays,
			event.OccurredAt,
			notif,
			exec,
		)
		if atomicErr != nil {
			errMsg := fmt.Sprintf("failed to execute rule atomically: %v", atomicErr)
			e.logger.Error("automation atomic execution error",
				logger.String("ruleId", rule.ID),
				logger.String("eventId", event.ID),
				logger.Error(atomicErr),
			)
			res.Status = StatusFailed
			res.Error = errMsg
			e.recordAuditFailure(ctx, rule, event, errMsg)
			return res
		}

		switch execStatus {
		case "skipped_duplicate":
			res.Status = StatusSkippedDuplicate
			return res
		case "skipped_cooldown":
			res.Status = StatusSkippedCooldown
			return res
		case "success":
			res.NotificationID = &notif.ID
			res.Status = StatusExecuted
		default:
			errMsg := fmt.Sprintf("unknown execution status returned: %q", execStatus)
			res.Status = StatusFailed
			res.Error = errMsg
			e.recordAuditFailure(ctx, rule, event, errMsg)
			return res
		}
	} else {
		// Fallback for mock environments without execRepo
		if e.directRepo != nil {
			if err := e.directRepo.Create(ctx, notif); err != nil {
				errMsg := fmt.Sprintf("failed to create direct notification: %v", err)
				res.Status = StatusFailed
				res.Error = errMsg
				e.recordAuditFailure(ctx, rule, event, errMsg)
				return res
			}
		}
		res.NotificationID = &notif.ID
		res.Status = StatusExecuted
	}

	// 7. Activity History / System Audit integration (automation.executed)
	if e.audit != nil {
		targetName := rule.Name
		details := map[string]any{
			"ruleId":         rule.ID,
			"ruleName":       rule.Name,
			"eventId":        event.ID,
			"eventType":      event.Type,
			"templateId":     tpl.ID,
			"templateKey":    tpl.TemplateKey,
			"notificationId": notif.ID,
			"recipientEmail": notif.RecipientEmail,
			"subject":        rendered.Subject,
		}

		_, auditErr := e.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventAutomationExecuted,
			Actor:      audit.SystemActor(),
			TargetType: domain.TargetTypeAutomationRule,
			TargetID:   rule.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    fmt.Sprintf("Executed automation rule %q for %s", rule.Name, notif.RecipientEmail),
			Details:    details,
		})
		if auditErr != nil {
			e.logger.Warn("failed to record automation execution audit event",
				logger.String("ruleId", rule.ID),
				logger.Error(auditErr),
			)
		}
	}

	return res
}

func (e *Engine) recordAuditFailure(ctx context.Context, rule *domain.AutomationRule, event Event, errMsg string) {
	if e.audit == nil || rule == nil {
		return
	}

	targetName := rule.Name
	details := map[string]any{
		"ruleId":       rule.ID,
		"ruleName":     rule.Name,
		"eventId":      event.ID,
		"eventType":    event.Type,
		"errorMessage": errMsg,
	}

	_, auditErr := e.audit.Record(ctx, audit.RecordInput{
		EventType:    domain.EventAutomationFailed,
		Actor:        audit.SystemActor(),
		TargetType:   domain.TargetTypeAutomationRule,
		TargetID:     rule.ID,
		TargetName:   &targetName,
		Status:       domain.ActivityStatusFailure,
		Summary:      fmt.Sprintf("Automation rule %q execution failed", rule.Name),
		ErrorMessage: &errMsg,
		Details:      details,
	})
	if auditErr != nil {
		e.logger.Warn("failed to record automation failure audit event",
			logger.String("ruleId", rule.ID),
			logger.Error(auditErr),
		)
	}
}

func buildContextData(event Event, user *userclient.User) map[string]any {
	ctx := make(map[string]any)

	// User fields
	if user != nil {
		ctx[domain.FieldUsername] = user.Username
		ctx[domain.FieldEmail] = user.Email
		ctx[domain.FieldRole] = user.Role
		ctx[domain.FieldLanguage] = user.Language
		ctx[domain.FieldIsActive] = user.IsActive
		ctx[domain.FieldIsBlocked] = user.IsBlocked
		ctx[domain.FieldRegistrationDate] = user.RegistrationDate
		if user.LastActivityAt != nil {
			ctx[domain.FieldLastActivityAt] = *user.LastActivityAt
			ctx[domain.FieldLastActivity] = *user.LastActivityAt
		} else {
			ctx[domain.FieldLastActivityAt] = nil
			ctx[domain.FieldLastActivity] = nil
		}
		ctx["userId"] = user.ID
		ctx["id"] = user.ID
	}

	// Event metadata
	ctx["eventId"] = event.ID
	ctx["eventType"] = event.Type
	ctx["occurredAt"] = event.OccurredAt

	// Event custom data (explicit payload overrides user defaults)
	for k, v := range event.Data {
		ctx[k] = v
	}

	return ctx
}

func extractRecipientEmail(user *userclient.User, event Event, contextData map[string]any) string {
	if user != nil && strings.TrimSpace(user.Email) != "" {
		return strings.TrimSpace(user.Email)
	}
	if em, ok := contextData[domain.FieldEmail].(string); ok && strings.TrimSpace(em) != "" {
		return strings.TrimSpace(em)
	}
	if em, ok := event.Data["recipientEmail"].(string); ok && strings.TrimSpace(em) != "" {
		return strings.TrimSpace(em)
	}
	return ""
}

func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return addr.Address == email && strings.Contains(email, "@") && strings.Contains(email, ".")
}
