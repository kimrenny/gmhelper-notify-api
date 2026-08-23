package direct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput     = errors.New("invalid direct notification input")
	ErrTemplateInactive = errors.New("template is not active for delivery")
	ErrMissingVariable  = errors.New("missing required template variable")
	ErrNotFound         = domain.ErrNotFound
)

type CreateInput struct {
	TemplateID       string                  `json:"templateId"`
	ExternalUserID   string                  `json:"externalUserId,omitempty"`
	RecipientEmail   string                  `json:"recipientEmail"`
	RecipientName    string                  `json:"recipientName,omitempty"`
	NotificationType domain.NotificationType `json:"notificationType,omitempty"`
	Payload          map[string]any          `json:"payload,omitempty"`
}

type CreateResult struct {
	Notification *domain.DirectNotification
	Rendered     *RenderedEmail
}

type Service struct {
	templateRepo domain.EmailTemplateRepository
	directRepo   domain.DirectNotificationRepository
}

func NewService(templateRepo domain.EmailTemplateRepository, directRepo domain.DirectNotificationRepository) *Service {
	return &Service{
		templateRepo: templateRepo,
		directRepo:   directRepo,
	}
}

func (s *Service) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}
	return s.directRepo.GetByID(ctx, id)
}

func (s *Service) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	return s.directRepo.ListPending(ctx)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	templateID := strings.TrimSpace(input.TemplateID)
	recipientEmail := strings.TrimSpace(input.RecipientEmail)
	recipientName := strings.TrimSpace(input.RecipientName)
	externalUserID := strings.TrimSpace(input.ExternalUserID)
	notificationType := input.NotificationType

	if templateID == "" || recipientEmail == "" {
		return nil, fmt.Errorf("%w: templateId and recipientEmail are required", ErrInvalidInput)
	}

	if !isValidEmail(recipientEmail) {
		return nil, fmt.Errorf("%w: invalid recipient email address '%s'", ErrInvalidInput, recipientEmail)
	}

	if notificationType == "" {
		notificationType = domain.NotificationTypeDirect
	}
	if !notificationType.IsValid() {
		return nil, fmt.Errorf("%w: invalid notification type '%s'", ErrInvalidInput, notificationType)
	}

	// 1. Resolve template
	tpl, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		return nil, err
	}

	// 2. Validate template status for delivery
	if tpl.Status != domain.TemplateStatusActive {
		return nil, fmt.Errorf("%w: template '%s' has status '%s'", ErrTemplateInactive, tpl.ID, tpl.Status)
	}

	// 3. Render email content with payload variables for validation
	rendered, err := RenderEmail(tpl.Subject, tpl.HTMLBody, tpl.PlainTextBody, input.Payload)
	if err != nil {
		return nil, err
	}

	var payloadBytes json.RawMessage
	if input.Payload != nil {
		bytes, err := json.Marshal(input.Payload)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to serialize payload: %v", ErrInvalidInput, err)
		}
		payloadBytes = bytes
	}

	now := time.Now().UTC()
	notification := &domain.DirectNotification{
		ID:               uuid.NewString(),
		TemplateID:       tpl.ID,
		ExternalUserID:   externalUserID,
		RecipientEmail:   recipientEmail,
		RecipientName:    recipientName,
		NotificationType: notificationType,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    0,
		Payload:          payloadBytes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	attempt := &domain.DeliveryAttempt{
		ID:            uuid.NewString(),
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notification.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		ErrorMessage:  "",
		AttemptedAt:   now,
		CreatedAt:     now,
	}

	// 4. Atomically persist notification and initial attempt
	if err := s.directRepo.CreateWithInitialAttempt(ctx, notification, attempt); err != nil {
		return nil, err
	}

	return &CreateResult{
		Notification: notification,
		Rendered:     rendered,
	}, nil
}

func isValidEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || !strings.Contains(parts[1], ".") {
		return false
	}
	return true
}
