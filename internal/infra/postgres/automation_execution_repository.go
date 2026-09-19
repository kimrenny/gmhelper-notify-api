package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

type AutomationExecutionRepository struct {
	db *sql.DB
}

func NewAutomationExecutionRepository(db *sql.DB) *AutomationExecutionRepository {
	return &AutomationExecutionRepository{db: db}
}

func (r *AutomationExecutionRepository) RecordExecution(ctx context.Context, exec *domain.AutomationExecution) error {
	if exec == nil {
		return errors.New("automation execution cannot be nil")
	}

	if exec.ID == "" {
		exec.ID = uuid.NewString()
	}
	if exec.ExecutedAt.IsZero() {
		exec.ExecutedAt = time.Now().UTC()
	}
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = exec.ExecutedAt
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO automation_executions (id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		exec.ID,
		exec.RuleID,
		exec.EventID,
		exec.RecipientEmail,
		exec.ExternalUserID,
		exec.NotificationID,
		exec.Status,
		exec.ExecutedAt,
		exec.CreatedAt,
	)
	return err
}

func (r *AutomationExecutionRepository) HasExecution(ctx context.Context, ruleID, eventID string) (bool, error) {
	ruleID = strings.TrimSpace(ruleID)
	eventID = strings.TrimSpace(eventID)
	if ruleID == "" || eventID == "" {
		return false, nil
	}

	var dummy int
	err := r.db.QueryRowContext(ctx, `
SELECT 1 FROM automation_executions
WHERE rule_id = $1 AND event_id = $2
LIMIT 1`, ruleID, eventID).Scan(&dummy)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *AutomationExecutionRepository) GetLastSuccessfulExecution(
	ctx context.Context,
	ruleID, recipientEmail string,
	externalUserID *string,
) (*domain.AutomationExecution, error) {
	ruleID = strings.TrimSpace(ruleID)
	recipientEmail = strings.TrimSpace(recipientEmail)

	var uid string
	if externalUserID != nil {
		uid = strings.TrimSpace(*externalUserID)
	}

	row := r.db.QueryRowContext(ctx, `
SELECT id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at
FROM automation_executions
WHERE rule_id = $1 AND status = 'success' AND (
	recipient_email = $2 OR ($3 != '' AND external_user_id = $3)
)
ORDER BY executed_at DESC
LIMIT 1`, ruleID, recipientEmail, uid)

	exec := &domain.AutomationExecution{}
	err := row.Scan(
		&exec.ID,
		&exec.RuleID,
		&exec.EventID,
		&exec.RecipientEmail,
		&exec.ExternalUserID,
		&exec.NotificationID,
		&exec.Status,
		&exec.ExecutedAt,
		&exec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return exec, nil
}

// ExecuteRuleAtomic enforces idempotency, cooldown checks, direct notification creation, and execution recording
// atomically inside a PostgreSQL transaction protected by a transaction-level advisory lock.
func (r *AutomationExecutionRepository) ExecuteRuleAtomic(
	ctx context.Context,
	ruleID, eventID, recipientEmail string,
	externalUserID *string,
	cooldownDays *int,
	eventTime time.Time,
	notif *domain.DirectNotification,
	exec *domain.AutomationExecution,
) (string, error) {
	ruleID = strings.TrimSpace(ruleID)
	eventID = strings.TrimSpace(eventID)
	recipientEmail = strings.TrimSpace(recipientEmail)

	if ruleID == "" || eventID == "" {
		return "failed", errors.New("rule_id and event_id are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "failed", err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Transaction-level PostgreSQL Advisory Lock to serialize concurrent events for the same rule + recipient.
	// hashtext produces a 32-bit integer; pg_advisory_xact_lock(int, int) takes two 32-bit integers.
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, ruleID, recipientEmail)
	if err != nil {
		return "failed", fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	// 2. Persistent Idempotency Check: Has this rule + event already executed?
	var existingStatus string
	err = tx.QueryRowContext(ctx, `
SELECT status FROM automation_executions
WHERE rule_id = $1 AND event_id = $2
LIMIT 1`, ruleID, eventID).Scan(&existingStatus)
	if err == nil {
		_ = tx.Commit()
		return "skipped_duplicate", nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "failed", fmt.Errorf("failed to check idempotency: %w", err)
	}

	// 3. Cooldown Check: If configured, verify the last successful execution for this recipient
	if cooldownDays != nil && *cooldownDays > 0 {
		cooldownDuration := time.Duration(*cooldownDays) * 24 * time.Hour
		var uid string
		if externalUserID != nil {
			uid = strings.TrimSpace(*externalUserID)
		}

		var lastExecutedAt time.Time
		err = tx.QueryRowContext(ctx, `
SELECT executed_at FROM automation_executions
WHERE rule_id = $1 AND status = 'success' AND (
	recipient_email = $2 OR ($3 != '' AND external_user_id = $3)
)
ORDER BY executed_at DESC
LIMIT 1`, ruleID, recipientEmail, uid).Scan(&lastExecutedAt)

		if err == nil {
			elapsed := eventTime.Sub(lastExecutedAt)
			if elapsed >= 0 && elapsed < cooldownDuration {
				skippedExecID := uuid.NewString()
				now := time.Now().UTC()
				if eventTime.IsZero() {
					eventTime = now
				}
				_, insertErr := tx.ExecContext(ctx, `
INSERT INTO automation_executions (id, rule_id, event_id, recipient_email, external_user_id, status, executed_at, created_at)
VALUES ($1, $2, $3, $4, $5, 'skipped_cooldown', $6, $7)`,
					skippedExecID, ruleID, eventID, recipientEmail, externalUserID, eventTime, now)
				if insertErr != nil {
					return "failed", fmt.Errorf("failed to record skipped cooldown: %w", insertErr)
				}

				if err := tx.Commit(); err != nil {
					return "failed", err
				}
				return "skipped_cooldown", nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "failed", fmt.Errorf("failed to check cooldown: %w", err)
		}
	}

	// 4. Create direct notification and record successful execution atomically
	if notif != nil {
		if err := notif.Validate(ctx); err != nil {
			return "failed", err
		}

		var payloadArg any
		if len(notif.Payload) > 0 {
			payloadArg = notif.Payload
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO direct_notifications (id, template_id, external_user_id, recipient_email, recipient_name, notification_type, delivery_status, attempts_count, last_attempt_at, sent_at, error_message, payload, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			notif.ID, notif.TemplateID, notif.ExternalUserID, notif.RecipientEmail, notif.RecipientName,
			notif.NotificationType, notif.DeliveryStatus, notif.AttemptsCount, notif.LastAttemptAt,
			notif.SentAt, notif.ErrorMessage, payloadArg, notif.CreatedAt, notif.UpdatedAt)
		if err != nil {
			return "failed", fmt.Errorf("failed to create direct notification: %w", err)
		}
	}

	if exec != nil {
		if exec.ID == "" {
			exec.ID = uuid.NewString()
		}
		if exec.ExecutedAt.IsZero() {
			exec.ExecutedAt = eventTime
		}
		if exec.CreatedAt.IsZero() {
			exec.CreatedAt = time.Now().UTC()
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO automation_executions (id, rule_id, event_id, recipient_email, external_user_id, notification_id, status, executed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			exec.ID, exec.RuleID, exec.EventID, exec.RecipientEmail, exec.ExternalUserID,
			exec.NotificationID, exec.Status, exec.ExecutedAt, exec.CreatedAt)
		if err != nil {
			return "failed", fmt.Errorf("failed to record execution: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "failed", err
	}

	return "success", nil
}
