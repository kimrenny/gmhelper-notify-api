package direct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput            = errors.New("invalid direct notification input")
	ErrTemplateInactive        = errors.New("template is not active for delivery")
	ErrMissingVariable         = errors.New("missing required template variable")
	ErrNotFound                = domain.ErrNotFound
	ErrUserResolverUnavailable = errors.New("user resolution service is unavailable")
)

// UserResolver defines the interface required by direct.Service to resolve external users.
type UserResolver interface {
	GetUserByID(ctx context.Context, id string) (*userclient.User, error)
}

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
	ResolvedUser *userclient.User
}

type directMessageDetails struct {
	ID               string                  `json:"id"`
	TemplateID       string                  `json:"templateId"`
	TemplateKey      string                  `json:"templateKey,omitempty"`
	TemplateName     string                  `json:"templateName,omitempty"`
	ExternalUserID   string                  `json:"externalUserId,omitempty"`
	RecipientEmail   string                  `json:"recipientEmail"`
	RecipientName    string                  `json:"recipientName,omitempty"`
	NotificationType domain.NotificationType `json:"notificationType"`
	Subject          string                  `json:"subject"`
	BodyHTML         string                  `json:"bodyHtml"`
	BodyPlain        string                  `json:"bodyPlain"`
}

type Service struct {
	templateRepo domain.EmailTemplateRepository
	directRepo   domain.DirectNotificationRepository
	userResolver UserResolver
	audit        *audit.Service
}

func NewService(templateRepo domain.EmailTemplateRepository, directRepo domain.DirectNotificationRepository, userResolver UserResolver, auditSvc *audit.Service) *Service {
	return &Service{
		templateRepo: templateRepo,
		directRepo:   directRepo,
		userResolver: userResolver,
		audit:        auditSvc,
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

	// 1. Resolve user if externalUserId is provided
	var resolvedUser *userclient.User
	if externalUserID != "" {
		if s.userResolver == nil {
			return nil, ErrUserResolverUnavailable
		}
		user, err := s.userResolver.GetUserByID(ctx, externalUserID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve external user %s: %w", externalUserID, err)
		}
		resolvedUser = user
	}

	// 2. Resolve template
	tpl, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		return nil, err
	}

	// 3. Validate template status for delivery
	if tpl.Status != domain.TemplateStatusActive {
		return nil, fmt.Errorf("%w: template '%s' has status '%s'", ErrTemplateInactive, tpl.ID, tpl.Status)
	}

	// 4. Merge resolved user variables with explicitly provided payload (explicit variables take precedence)
	renderPayload := input.Payload
	if resolvedUser != nil {
		renderPayload = MergeUserPayload(resolvedUser, input.Payload)
	}

	// 5. Render email content with payload variables for validation
	rendered, err := RenderEmail(tpl.Subject, tpl.HTMLBody, tpl.PlainTextBody, renderPayload)
	if err != nil {
		return nil, err
	}

	var payloadBytes json.RawMessage
	if renderPayload != nil {
		bytes, err := json.Marshal(renderPayload)
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

	// 5. Atomically persist notification and initial attempt
	if err := s.directRepo.CreateWithInitialAttempt(ctx, notification, attempt); err != nil {
		return nil, err
	}

	if s.audit != nil {
		actor, err := audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}

		summary := fmt.Sprintf("Created direct message to %s", notification.RecipientEmail)
		_, auditErr := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventDirectCreated,
			Actor:      actor,
			TargetType: domain.TargetTypeDirectNotification,
			TargetID:   notification.ID,
			TargetName: &notification.RecipientEmail,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details: directMessageDetails{
				ID:               notification.ID,
				TemplateID:       tpl.ID,
				TemplateKey:      tpl.TemplateKey,
				TemplateName:     tpl.Name,
				ExternalUserID:   externalUserID,
				RecipientEmail:   recipientEmail,
				RecipientName:    recipientName,
				NotificationType: notificationType,
				Subject:          rendered.Subject,
				BodyHTML:         rendered.HTMLBody,
				BodyPlain:        rendered.PlainTextBody,
			},
		})
		if auditErr != nil {
			return nil, auditErr
		}
	}

	return &CreateResult{
		Notification: notification,
		Rendered:     rendered,
		ResolvedUser: resolvedUser,
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

// MergeUserPayload maps approved user fields (username, email, role, language) into the template payload.
// Explicit caller-provided payload variables take precedence over resolved user fields.
func MergeUserPayload(u *userclient.User, payload map[string]any) map[string]any {
	if u == nil {
		return payload
	}

	merged := make(map[string]any, len(payload)+4)
	merged["username"] = u.Username
	merged["email"] = u.Email
	merged["role"] = u.Role
	merged["language"] = u.Language

	for k, v := range payload {
		merged[k] = v
	}

	return merged
}
