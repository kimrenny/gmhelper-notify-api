package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/google/uuid"
)

var (
	ErrRecipientNil          = errors.New("campaign recipient cannot be nil")
	ErrTemplateInactive      = errors.New("referenced template is inactive")
	ErrCampaignNotExecutable = errors.New("referenced campaign is not in running state")
)

type campaignCompletedDetails struct {
	CampaignID      string                `json:"campaignId"`
	CampaignName    string                `json:"campaignName"`
	Status          domain.CampaignStatus `json:"status"`
	TotalRecipients int                   `json:"totalRecipients"`
	SentCount       int                   `json:"sentCount"`
	FailedCount     int                   `json:"failedCount"`
	CompletedAt     time.Time             `json:"completedAt"`
}

type campaignFailedDetails struct {
	CampaignID      string                `json:"campaignId"`
	CampaignName    string                `json:"campaignName"`
	Status          domain.CampaignStatus `json:"status"`
	TotalRecipients int                   `json:"totalRecipients"`
	SentCount       int                   `json:"sentCount"`
	FailedCount     int                   `json:"failedCount"`
	CompletedAt     time.Time             `json:"completedAt"`
	ErrorMessage    string                `json:"errorMessage"`
}

// DeliveryService delivers claimed campaign recipients and manages campaign lifecycle finalization.
type DeliveryService struct {
	campaignRepo  domain.NotificationCampaignRepository
	recipientRepo domain.CampaignRecipientRepository
	templateRepo  domain.EmailTemplateRepository
	attemptRepo   domain.DeliveryAttemptRepository
	sender        email.Sender
	userResolver  user.UserResolver
	logger        logger.Logger
	audit         *audit.Service
}

// NewDeliveryService constructs a new DeliveryService for campaign recipients.
func NewDeliveryService(
	campaignRepo domain.NotificationCampaignRepository,
	recipientRepo domain.CampaignRecipientRepository,
	templateRepo domain.EmailTemplateRepository,
	attemptRepo domain.DeliveryAttemptRepository,
	sender email.Sender,
	userResolver user.UserResolver,
	auditSvc *audit.Service,
) *DeliveryService {
	return NewDeliveryServiceWithLogger(campaignRepo, recipientRepo, templateRepo, attemptRepo, sender, userResolver, nil, auditSvc)
}

// NewDeliveryServiceWithLogger constructs a new DeliveryService with an optional logger.
func NewDeliveryServiceWithLogger(
	campaignRepo domain.NotificationCampaignRepository,
	recipientRepo domain.CampaignRecipientRepository,
	templateRepo domain.EmailTemplateRepository,
	attemptRepo domain.DeliveryAttemptRepository,
	sender email.Sender,
	userResolver user.UserResolver,
	log logger.Logger,
	auditSvc *audit.Service,
) *DeliveryService {
	return &DeliveryService{
		campaignRepo:  campaignRepo,
		recipientRepo: recipientRepo,
		templateRepo:  templateRepo,
		attemptRepo:   attemptRepo,
		sender:        sender,
		userResolver:  userResolver,
		logger:        log,
		audit:         auditSvc,
	}
}

// DeliverClaimed processes a single claimed recipient (in 'sending' status) and executes template rendering, user personalization, and SMTP delivery.
func (s *DeliveryService) DeliverClaimed(ctx context.Context, recipient *domain.CampaignRecipient) error {
	if recipient == nil {
		return ErrRecipientNil
	}

	now := time.Now().UTC()
	attemptsCount := recipient.AttemptsCount
	if attemptsCount <= 0 {
		attemptsCount = 1
	}

	// 1. Load referenced campaign
	cmp, err := s.campaignRepo.GetByID(ctx, recipient.CampaignID)
	if err != nil {
		s.recordImmediateFailure(ctx, recipient, attemptsCount, now, fmt.Sprintf("failed to load campaign: %v", err))
		return err
	}

	// 2. Load referenced template
	tpl, err := s.templateRepo.GetByID(ctx, cmp.TemplateID)
	if err != nil {
		s.recordImmediateFailure(ctx, recipient, attemptsCount, now, fmt.Sprintf("failed to load template: %v", err))
		return err
	}
	if tpl.Status != domain.TemplateStatusActive {
		err = fmt.Errorf("%w: template '%s' status is '%s'", ErrTemplateInactive, tpl.ID, tpl.Status)
		s.recordImmediateFailure(ctx, recipient, attemptsCount, now, err.Error())
		return err
	}

	// 3. Resolve user variables if external user ID is present
	vars := map[string]any{
		"username": recipient.RecipientName,
		"email":    recipient.RecipientEmail,
	}
	if s.userResolver != nil && strings.TrimSpace(recipient.ExternalUserID) != "" {
		u, uErr := s.userResolver.GetUserByID(ctx, recipient.ExternalUserID)
		if uErr == nil && u != nil {
			vars = direct.MergeUserPayload(u, nil)
		}
	}

	// 4. Render template
	rendered, err := direct.RenderEmail(tpl.Subject, tpl.HTMLBody, tpl.PlainTextBody, vars)
	if err != nil {
		s.recordImmediateFailure(ctx, recipient, attemptsCount, now, fmt.Sprintf("template rendering failed: %v", err))
		return err
	}

	// 5. Initialize DeliveryAttempt
	attempt := &domain.DeliveryAttempt{
		ID:            uuid.NewString(),
		TargetType:    domain.DeliveryTargetCampaignRecipient,
		TargetID:      recipient.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: attemptsCount,
		AttemptedAt:   now,
		CreatedAt:     now,
	}
	_ = s.attemptRepo.Create(ctx, attempt)

	// 6. Send email via SMTP
	msg := &email.Message{
		To:            recipient.RecipientEmail,
		Subject:       rendered.Subject,
		HTMLBody:      rendered.HTMLBody,
		PlainTextBody: rendered.PlainTextBody,
	}

	if s.logger != nil {
		s.logger.Info("starting campaign recipient email delivery",
			logger.String("recipientId", recipient.ID),
			logger.String("campaignId", recipient.CampaignID),
			logger.String("recipientEmail", recipient.RecipientEmail),
			logger.Int("attemptNumber", attemptsCount),
		)
	}

	sendErr := s.sender.Send(ctx, msg)

	// 7. Record delivery outcome
	if sendErr == nil {
		sentAt := time.Now().UTC()
		_ = s.recipientRepo.UpdateStatus(ctx, recipient.ID, domain.DeliveryStatusSent, attemptsCount, &sentAt, &sentAt, "")

		attempt.Status = domain.DeliveryStatusSent
		attempt.ErrorMessage = ""
		attempt.AttemptedAt = sentAt
		_ = s.attemptRepo.Update(ctx, attempt)

		if s.logger != nil {
			s.logger.Info("campaign recipient delivered successfully via SMTP",
				logger.String("recipientId", recipient.ID),
				logger.String("campaignId", recipient.CampaignID),
				logger.String("recipientEmail", recipient.RecipientEmail),
				logger.String("subject", rendered.Subject),
				logger.String("attemptId", attempt.ID),
			)
		}
		return nil
	}

	failedAt := time.Now().UTC()
	errMsg := sendErr.Error()
	_ = s.recipientRepo.UpdateStatus(ctx, recipient.ID, domain.DeliveryStatusFailed, attemptsCount, &failedAt, nil, errMsg)

	attempt.Status = domain.DeliveryStatusFailed
	attempt.ErrorMessage = errMsg
	attempt.AttemptedAt = failedAt
	_ = s.attemptRepo.Update(ctx, attempt)

	if s.logger != nil {
		s.logger.Error("campaign recipient SMTP delivery failed",
			logger.String("recipientId", recipient.ID),
			logger.String("campaignId", recipient.CampaignID),
			logger.String("recipientEmail", recipient.RecipientEmail),
			logger.Int("attemptNumber", attemptsCount),
			logger.Error(sendErr),
		)
	}

	return sendErr
}

// FinalizeCampaignIfDone checks if all recipients for the given campaign have completed (none pending or sending) and transitions the campaign to a terminal state.
func (s *DeliveryService) FinalizeCampaignIfDone(ctx context.Context, campaignID string) (bool, domain.CampaignStatus, error) {
	camp, err := s.campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		return false, "", err
	}

	// If campaign is already in a terminal state (completed, partially_failed, failed, cancelled),
	// do not attempt to finalize again and do not emit duplicate audit events.
	if camp.Status == domain.CampaignStatusCompleted ||
		camp.Status == domain.CampaignStatusPartiallyFailed ||
		camp.Status == domain.CampaignStatusFailed ||
		camp.Status == domain.CampaignStatusCancelled {
		return false, camp.Status, nil
	}

	stats, err := s.recipientRepo.GetDeliveryStatsByCampaign(ctx, campaignID)
	if err != nil {
		return false, "", err
	}

	// If no recipients exist yet, population is still in progress (or managed by AudiencePopulator).
	// Delivery runner must not prematurely finalize campaigns with zero recipients.
	if stats.TotalCount == 0 {
		return false, camp.Status, nil
	}

	// If any recipient is still pending or currently sending, do not finalize
	if stats.PendingCount > 0 || stats.SendingCount > 0 {
		return false, camp.Status, nil
	}

	var finalStatus domain.CampaignStatus
	switch {
	case stats.SentCount > 0 && stats.FailedCount == 0:
		finalStatus = domain.CampaignStatusCompleted
	case stats.SentCount > 0 && stats.FailedCount > 0:
		finalStatus = domain.CampaignStatusPartiallyFailed
	case stats.SentCount == 0 && stats.FailedCount > 0:
		finalStatus = domain.CampaignStatusFailed
	default:
		finalStatus = domain.CampaignStatusFailed
	}

	completedAt := time.Now().UTC()
	if err := s.campaignRepo.UpdateStatus(ctx, campaignID, finalStatus, nil, &completedAt); err != nil {
		return false, "", err
	}

	if s.audit != nil {
		campaignName := camp.Name
		if campaignName == "" {
			campaignName = campaignID
		}

		if finalStatus == domain.CampaignStatusCompleted || finalStatus == domain.CampaignStatusPartiallyFailed {
			status := domain.ActivityStatusSuccess
			summary := fmt.Sprintf("Campaign %q completed execution", campaignName)
			if finalStatus == domain.CampaignStatusPartiallyFailed {
				status = domain.ActivityStatusWarning
				summary = fmt.Sprintf("Campaign %q completed with partial failures", campaignName)
			}
			if _, auditErr := s.audit.Record(ctx, audit.RecordInput{
				EventType:  domain.EventCampaignCompleted,
				Actor:      audit.SystemActor(),
				TargetType: domain.TargetTypeCampaign,
				TargetID:   campaignID,
				TargetName: &campaignName,
				Status:     status,
				Summary:    summary,
				Details: campaignCompletedDetails{
					CampaignID:      campaignID,
					CampaignName:    campaignName,
					Status:          finalStatus,
					TotalRecipients: stats.TotalCount,
					SentCount:       stats.SentCount,
					FailedCount:     stats.FailedCount,
					CompletedAt:     completedAt,
				},
			}); auditErr != nil {
				if s.logger != nil {
					s.logger.Error("failed to record campaign completed audit log",
						logger.String("campaignId", campaignID),
						logger.Error(auditErr),
					)
				}
			}
		} else if finalStatus == domain.CampaignStatusFailed {
			errMsg := "all recipient deliveries failed"
			summary := fmt.Sprintf("Campaign %q failed execution", campaignName)
			if _, auditErr := s.audit.Record(ctx, audit.RecordInput{
				EventType:    domain.EventCampaignFailed,
				Actor:        audit.SystemActor(),
				TargetType:   domain.TargetTypeCampaign,
				TargetID:     campaignID,
				TargetName:   &campaignName,
				Status:       domain.ActivityStatusFailure,
				Summary:      summary,
				ErrorMessage: &errMsg,
				Details: campaignFailedDetails{
					CampaignID:      campaignID,
					CampaignName:    campaignName,
					Status:          finalStatus,
					TotalRecipients: stats.TotalCount,
					SentCount:       stats.SentCount,
					FailedCount:     stats.FailedCount,
					CompletedAt:     completedAt,
					ErrorMessage:    errMsg,
				},
			}); auditErr != nil {
				if s.logger != nil {
					s.logger.Error("failed to record campaign failed audit log",
						logger.String("campaignId", campaignID),
						logger.Error(auditErr),
					)
				}
			}
		}
	}

	return true, finalStatus, nil
}

func (s *DeliveryService) recordImmediateFailure(ctx context.Context, recipient *domain.CampaignRecipient, attemptsCount int, now time.Time, errMsg string) {
	_ = s.recipientRepo.UpdateStatus(ctx, recipient.ID, domain.DeliveryStatusFailed, attemptsCount, &now, nil, errMsg)

	attempt := &domain.DeliveryAttempt{
		ID:            uuid.NewString(),
		TargetType:    domain.DeliveryTargetCampaignRecipient,
		TargetID:      recipient.ID,
		Status:        domain.DeliveryStatusFailed,
		AttemptNumber: attemptsCount,
		ErrorMessage:  errMsg,
		AttemptedAt:   now,
		CreatedAt:     now,
	}
	_ = s.attemptRepo.Create(ctx, attempt)
}
