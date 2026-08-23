package direct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrAlreadySent           = errors.New("direct notification is already sent")
	ErrNotificationCancelled = errors.New("direct notification is cancelled")
	ErrInvalidDeliveryState  = errors.New("direct notification is not in pending state")
)

type DeliveryService struct {
	directRepo   domain.DirectNotificationRepository
	attemptRepo  domain.DeliveryAttemptRepository
	templateRepo domain.EmailTemplateRepository
	sender       email.Sender
}

func NewDeliveryService(
	directRepo domain.DirectNotificationRepository,
	attemptRepo domain.DeliveryAttemptRepository,
	templateRepo domain.EmailTemplateRepository,
	sender email.Sender,
) *DeliveryService {
	return &DeliveryService{
		directRepo:   directRepo,
		attemptRepo:  attemptRepo,
		templateRepo: templateRepo,
		sender:       sender,
	}
}

// Deliver delivers a pending direct notification via SMTP and records delivery outcomes.
func (s *DeliveryService) Deliver(ctx context.Context, notificationID string) error {
	notificationID = strings.TrimSpace(notificationID)
	if notificationID == "" {
		return fmt.Errorf("%w: notification id is required", ErrInvalidInput)
	}

	// 1. Load DirectNotification
	notification, err := s.directRepo.GetByID(ctx, notificationID)
	if err != nil {
		return err
	}

	// 2. Validate current delivery state
	switch notification.DeliveryStatus {
	case domain.DeliveryStatusSent:
		return ErrAlreadySent
	case domain.DeliveryStatusCancelled:
		return ErrNotificationCancelled
	case domain.DeliveryStatusPending:
		// Expected state
	default:
		return fmt.Errorf("%w: current status is '%s'", ErrInvalidDeliveryState, notification.DeliveryStatus)
	}

	// 3. Load referenced template
	tpl, err := s.templateRepo.GetByID(ctx, notification.TemplateID)
	if err != nil {
		return err
	}
	if tpl.Status != domain.TemplateStatusActive {
		return fmt.Errorf("%w: template '%s' is '%s'", ErrTemplateInactive, tpl.ID, tpl.Status)
	}

	// 4. Render email content
	var vars map[string]any
	if len(notification.Payload) > 0 {
		if err := json.Unmarshal(notification.Payload, &vars); err != nil {
			return fmt.Errorf("%w: failed to parse notification payload: %v", ErrInvalidInput, err)
		}
	}

	rendered, err := RenderEmail(tpl.Subject, tpl.HTMLBody, tpl.PlainTextBody, vars)
	if err != nil {
		return err
	}

	// 5. Mark notification as sending
	now := time.Now().UTC()
	attemptsCount := notification.AttemptsCount + 1
	if err := s.directRepo.UpdateStatus(ctx, notification.ID, domain.DeliveryStatusSending, attemptsCount, &now, nil, ""); err != nil {
		return fmt.Errorf("failed to update notification to sending: %w", err)
	}

	// 6. Find or initialize delivery attempt record
	targetAttempt, err := s.findOrInitAttempt(ctx, notification.ID, attemptsCount, now)
	if err != nil {
		return fmt.Errorf("failed to prepare delivery attempt record: %w", err)
	}

	// 7. Send email via SMTP (outside any database transaction)
	msg := &email.Message{
		To:            notification.RecipientEmail,
		Subject:       rendered.Subject,
		HTMLBody:      rendered.HTMLBody,
		PlainTextBody: rendered.PlainTextBody,
	}

	sendErr := s.sender.Send(ctx, msg)

	// 8. Persist delivery outcome
	if sendErr == nil {
		sentAt := time.Now().UTC()
		if err := s.directRepo.UpdateStatus(ctx, notification.ID, domain.DeliveryStatusSent, attemptsCount, &sentAt, &sentAt, ""); err != nil {
			return fmt.Errorf("failed to record successful delivery status: %w", err)
		}

		targetAttempt.Status = domain.DeliveryStatusSent
		targetAttempt.ErrorMessage = ""
		targetAttempt.AttemptedAt = sentAt
		if err := s.attemptRepo.Update(ctx, targetAttempt); err != nil {
			return fmt.Errorf("failed to update successful delivery attempt: %w", err)
		}
		return nil
	}

	// Delivery failure handling
	failedAt := time.Now().UTC()
	errMsg := sendErr.Error()
	if err := s.directRepo.UpdateStatus(ctx, notification.ID, domain.DeliveryStatusFailed, attemptsCount, &failedAt, nil, errMsg); err != nil {
		return fmt.Errorf("failed to record failed delivery status: %w (original error: %v)", err, sendErr)
	}

	targetAttempt.Status = domain.DeliveryStatusFailed
	targetAttempt.ErrorMessage = errMsg
	targetAttempt.AttemptedAt = failedAt
	if err := s.attemptRepo.Update(ctx, targetAttempt); err != nil {
		return fmt.Errorf("failed to update failed delivery attempt: %w (original error: %v)", err, sendErr)
	}

	return sendErr
}

func (s *DeliveryService) findOrInitAttempt(ctx context.Context, targetID string, attemptNumber int, now time.Time) (*domain.DeliveryAttempt, error) {
	attempts, err := s.attemptRepo.ListByTarget(ctx, domain.DeliveryTargetDirectNotification, targetID)
	if err != nil {
		return nil, err
	}

	for _, att := range attempts {
		if att.Status == domain.DeliveryStatusPending {
			return att, nil
		}
	}

	newAttempt := &domain.DeliveryAttempt{
		ID:            uuid.NewString(),
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      targetID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: attemptNumber,
		AttemptedAt:   now,
		CreatedAt:     now,
	}

	if err := s.attemptRepo.Create(ctx, newAttempt); err != nil {
		return nil, err
	}
	return newAttempt, nil
}
