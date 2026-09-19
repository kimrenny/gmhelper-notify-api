package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid campaign input")
	ErrNotFound     = domain.ErrNotFound
	ErrInvalidState = errors.New("invalid campaign status for this operation")
)

type CreateInput struct {
	Name         string
	TemplateID   string
	CampaignType string
	Status       string
	ScheduledAt  *time.Time
}

type UpdateInput struct {
	Name         *string
	TemplateID   *string
	CampaignType *string
	Status       *string
	ScheduledAt  *time.Time
}

type campaignCreatedDetails struct {
	CampaignID      string                `json:"campaignId"`
	CampaignName    string                `json:"campaignName"`
	CampaignType    string                `json:"campaignType"`
	TemplateID      string                `json:"templateId"`
	TemplateKey     string                `json:"templateKey,omitempty"`
	TemplateName    string                `json:"templateName,omitempty"`
	TemplateLocale  string                `json:"templateLocale,omitempty"`
	TemplateVersion int                   `json:"templateVersion,omitempty"`
	Subject         string                `json:"subject,omitempty"`
	BodyHTML        string                `json:"bodyHtml,omitempty"`
	BodyPlain       string                `json:"bodyPlain,omitempty"`
	Locale          string                `json:"locale,omitempty"`
	InitialStatus   domain.CampaignStatus `json:"initialStatus"`
	Status          domain.CampaignStatus `json:"status"`
	ScheduledAt     *time.Time            `json:"scheduledAt,omitempty"`
}

type campaignScheduledDetails struct {
	CampaignID      string                `json:"campaignId"`
	CampaignName    string                `json:"campaignName"`
	ScheduledAt     time.Time             `json:"scheduledAt"`
	PreviousStatus  domain.CampaignStatus `json:"previousStatus"`
	ResultingStatus domain.CampaignStatus `json:"resultingStatus"`
	Status          domain.CampaignStatus `json:"status"`
}

type campaignCancelledDetails struct {
	CampaignID      string                `json:"campaignId"`
	CampaignName    string                `json:"campaignName"`
	PreviousStatus  domain.CampaignStatus `json:"previousStatus"`
	ResultingStatus domain.CampaignStatus `json:"resultingStatus"`
	Status          domain.CampaignStatus `json:"status"`
	CancelledAt     time.Time             `json:"cancelledAt"`
}

type Service struct {
	repo         domain.NotificationCampaignRepository
	templateRepo domain.EmailTemplateRepository
	audit        *audit.Service
}

func NewService(repo domain.NotificationCampaignRepository, templateRepo domain.EmailTemplateRepository, auditSvc *audit.Service) *Service {
	return &Service{
		repo:         repo,
		templateRepo: templateRepo,
		audit:        auditSvc,
	}
}

func (s *Service) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.NotificationCampaign, error) {
	name := strings.TrimSpace(input.Name)
	templateID := strings.TrimSpace(input.TemplateID)
	campaignType := strings.TrimSpace(input.CampaignType)

	if name == "" || templateID == "" {
		return nil, ErrInvalidInput
	}

	if campaignType == "" {
		campaignType = "broadcast"
	}

	statusStr := strings.TrimSpace(input.Status)
	if statusStr != "" && statusStr != string(domain.CampaignStatusDraft) {
		return nil, ErrInvalidInput
	}
	status := domain.CampaignStatusDraft

	now := time.Now().UTC()
	var scheduledAt *time.Time
	if input.ScheduledAt != nil {
		t := input.ScheduledAt.UTC()
		scheduledAt = &t
	}

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	campaign := &domain.NotificationCampaign{
		ID:           uuid.NewString(),
		Name:         name,
		TemplateID:   templateID,
		CampaignType: campaignType,
		Status:       status,
		ScheduledAt:  scheduledAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, err
	}

	if s.audit != nil {
		var (
			tplKey     string
			tplName    string
			tplLocale  string
			tplVersion int
			subject    string
			bodyHTML   string
			bodyPlain  string
		)
		if s.templateRepo != nil {
			if tpl, err := s.templateRepo.GetByID(ctx, templateID); err == nil && tpl != nil {
				tplKey = tpl.TemplateKey
				tplName = tpl.Name
				tplLocale = tpl.Locale
				tplVersion = tpl.Version
				subject = tpl.Subject
				bodyHTML = tpl.HTMLBody
				bodyPlain = tpl.PlainTextBody
			}
		}

		summary := fmt.Sprintf("Created campaign %q", campaign.Name)
		_, auditErr := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventCampaignCreated,
			Actor:      actor,
			TargetType: domain.TargetTypeCampaign,
			TargetID:   campaign.ID,
			TargetName: &campaign.Name,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details: campaignCreatedDetails{
				CampaignID:      campaign.ID,
				CampaignName:    campaign.Name,
				CampaignType:    campaign.CampaignType,
				TemplateID:      campaign.TemplateID,
				TemplateKey:     tplKey,
				TemplateName:    tplName,
				TemplateLocale:  tplLocale,
				TemplateVersion: tplVersion,
				Subject:         subject,
				BodyHTML:        bodyHTML,
				BodyPlain:       bodyPlain,
				Locale:          tplLocale,
				InitialStatus:   campaign.Status,
				Status:          campaign.Status,
				ScheduledAt:     campaign.ScheduledAt,
			},
		})
		if auditErr != nil {
			return nil, auditErr
		}
	}

	return campaign, nil
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*domain.NotificationCampaign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrInvalidInput
		}
		existing.Name = name
	}

	if input.TemplateID != nil {
		templateID := strings.TrimSpace(*input.TemplateID)
		if templateID == "" {
			return nil, ErrInvalidInput
		}
		existing.TemplateID = templateID
	}

	if input.CampaignType != nil {
		campaignType := strings.TrimSpace(*input.CampaignType)
		if campaignType == "" {
			return nil, ErrInvalidInput
		}
		existing.CampaignType = campaignType
	}

	if input.Status != nil {
		statusStr := strings.TrimSpace(*input.Status)
		if statusStr != string(existing.Status) {
			return nil, ErrInvalidInput
		}
	}

	if input.ScheduledAt != nil {
		t := input.ScheduledAt.UTC()
		existing.ScheduledAt = &t
	}

	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *Service) Schedule(ctx context.Context, id string, scheduledAt time.Time) (*domain.NotificationCampaign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	if scheduledAt.IsZero() {
		return nil, ErrInvalidInput
	}

	if scheduledAt.Before(time.Now().UTC()) {
		return nil, ErrInvalidInput
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if existing.Status != domain.CampaignStatusDraft {
		return nil, ErrInvalidState
	}

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	utcTime := scheduledAt.UTC()
	existing.ScheduledAt = &utcTime
	existing.Status = domain.CampaignStatusScheduled
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	if s.audit != nil {
		summary := fmt.Sprintf("Scheduled campaign %q", existing.Name)
		_, auditErr := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventCampaignScheduled,
			Actor:      actor,
			TargetType: domain.TargetTypeCampaign,
			TargetID:   existing.ID,
			TargetName: &existing.Name,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details: campaignScheduledDetails{
				CampaignID:      existing.ID,
				CampaignName:    existing.Name,
				ScheduledAt:     utcTime,
				PreviousStatus:  domain.CampaignStatusDraft,
				ResultingStatus: domain.CampaignStatusScheduled,
				Status:          domain.CampaignStatusScheduled,
			},
		})
		if auditErr != nil {
			return nil, auditErr
		}
	}

	return existing, nil
}

func (s *Service) Cancel(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if existing.Status != domain.CampaignStatusScheduled {
		return nil, ErrInvalidState
	}

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	existing.Status = domain.CampaignStatusCancelled
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	if s.audit != nil {
		summary := fmt.Sprintf("Cancelled campaign %q", existing.Name)
		_, auditErr := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventCampaignCancelled,
			Actor:      actor,
			TargetType: domain.TargetTypeCampaign,
			TargetID:   existing.ID,
			TargetName: &existing.Name,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details: campaignCancelledDetails{
				CampaignID:      existing.ID,
				CampaignName:    existing.Name,
				PreviousStatus:  domain.CampaignStatusScheduled,
				ResultingStatus: domain.CampaignStatusCancelled,
				Status:          domain.CampaignStatusCancelled,
				CancelledAt:     existing.UpdatedAt,
			},
		})
		if auditErr != nil {
			return nil, auditErr
		}
	}

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidInput
	}
	return s.repo.Delete(ctx, id)
}
