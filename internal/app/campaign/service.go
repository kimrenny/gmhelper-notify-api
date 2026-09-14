package campaign

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid campaign input")
	ErrNotFound     = domain.ErrNotFound
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

type Service struct {
	repo domain.NotificationCampaignRepository
}

func NewService(repo domain.NotificationCampaignRepository) *Service {
	return &Service{repo: repo}
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
	statusStr := strings.TrimSpace(input.Status)

	if name == "" || templateID == "" {
		return nil, ErrInvalidInput
	}

	if campaignType == "" {
		campaignType = "broadcast"
	}

	status := domain.CampaignStatusDraft
	if statusStr != "" {
		status = domain.CampaignStatus(statusStr)
		if !status.IsValid() {
			return nil, ErrInvalidInput
		}
	}

	now := time.Now().UTC()
	scheduledAt := now
	if input.ScheduledAt != nil {
		scheduledAt = *input.ScheduledAt
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
		cType := strings.TrimSpace(*input.CampaignType)
		if cType != "" {
			existing.CampaignType = cType
		}
	}

	if input.Status != nil {
		statusStr := strings.TrimSpace(*input.Status)
		if statusStr != "" {
			status := domain.CampaignStatus(statusStr)
			if !status.IsValid() {
				return nil, ErrInvalidInput
			}
			existing.Status = status
		}
	}

	if input.ScheduledAt != nil {
		existing.ScheduledAt = *input.ScheduledAt
	}

	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
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
