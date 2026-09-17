package agreement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput        = errors.New("invalid agreement broadcast input")
	ErrTemplateNotFound    = errors.New("referenced template not found")
	ErrTemplateInactive    = errors.New("referenced template is not active")
	ErrBroadcastInProgress = errors.New("a user agreement broadcast is already in progress")
)

type CreateBroadcastInput struct {
	TemplateID string `json:"templateId"`
	Name       string `json:"name,omitempty"`
}

type Service struct {
	campaignRepo domain.NotificationCampaignRepository
	templateRepo domain.EmailTemplateRepository
}

func NewService(campaignRepo domain.NotificationCampaignRepository, templateRepo domain.EmailTemplateRepository) *Service {
	return &Service{
		campaignRepo: campaignRepo,
		templateRepo: templateRepo,
	}
}

// CreateBroadcast creates a new User Agreement broadcast campaign scheduled for immediate execution by the campaign pipeline.
func (s *Service) CreateBroadcast(ctx context.Context, input CreateBroadcastInput) (*domain.NotificationCampaign, error) {
	if s.campaignRepo == nil {
		return nil, errors.New("campaign repository is nil")
	}

	templateID := strings.TrimSpace(input.TemplateID)
	if templateID == "" {
		return nil, fmt.Errorf("%w: templateId is required", ErrInvalidInput)
	}

	// 1. Verify template exists and is active
	if s.templateRepo == nil {
		return nil, errors.New("template repository is nil")
	}

	tpl, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	if tpl == nil {
		return nil, ErrTemplateNotFound
	}

	if tpl.Status != domain.TemplateStatusActive {
		return nil, fmt.Errorf("%w: template '%s' status is '%s'", ErrTemplateInactive, tpl.ID, tpl.Status)
	}

	// 2. Safety check: prevent duplicate simultaneous agreement broadcasts
	activeStatuses := []domain.CampaignStatus{
		domain.CampaignStatusScheduled,
		domain.CampaignStatusRunning,
		domain.CampaignStatusSending,
	}

	for _, status := range activeStatuses {
		activeCampaigns, err := s.campaignRepo.ListByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing agreement broadcasts: %w", err)
		}
		for _, c := range activeCampaigns {
			if strings.EqualFold(c.CampaignType, string(domain.NotificationTypeUserAgreement)) {
				return nil, ErrBroadcastInProgress
			}
		}
	}

	// 3. Prepare campaign name
	name := strings.TrimSpace(input.Name)
	if name == "" {
		if tpl.Name != "" {
			name = "User Agreement - " + tpl.Name
		} else {
			name = "User Agreement Broadcast"
		}
	}

	now := time.Now().UTC()
	scheduledAt := now

	campaign := &domain.NotificationCampaign{
		ID:           uuid.NewString(),
		Name:         name,
		TemplateID:   templateID,
		CampaignType: string(domain.NotificationTypeUserAgreement),
		Status:       domain.CampaignStatusScheduled,
		ScheduledAt:  &scheduledAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.campaignRepo.Create(ctx, campaign); err != nil {
		return nil, err
	}

	return campaign, nil
}
