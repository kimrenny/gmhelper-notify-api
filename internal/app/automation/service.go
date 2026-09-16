package automation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput     = errors.New("invalid automation rule input")
	ErrNotFound         = domain.ErrNotFound
	ErrTemplateNotFound = errors.New("referenced template not found")
)

type CreateInput struct {
	Name       string                      `json:"name"`
	TemplateID string                      `json:"templateId"`
	Enabled    *bool                       `json:"enabled,omitempty"`
	Config     domain.AutomationRuleConfig `json:"config"`
}

type UpdateInput struct {
	Name       *string                      `json:"name,omitempty"`
	TemplateID *string                      `json:"templateId,omitempty"`
	Enabled    *bool                        `json:"enabled,omitempty"`
	Config     *domain.AutomationRuleConfig `json:"config,omitempty"`
}

type Service struct {
	repo         domain.AutomationRuleRepository
	templateRepo domain.EmailTemplateRepository
}

func NewService(repo domain.AutomationRuleRepository, templateRepo domain.EmailTemplateRepository) *Service {
	return &Service{
		repo:         repo,
		templateRepo: templateRepo,
	}
}

func (s *Service) List(ctx context.Context) ([]*domain.AutomationRule, error) {
	if s.repo == nil {
		return nil, errors.New("automation rule repository is nil")
	}
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	if s.repo == nil {
		return nil, errors.New("automation rule repository is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.AutomationRule, error) {
	if s.repo == nil {
		return nil, errors.New("automation rule repository is nil")
	}

	name := strings.TrimSpace(input.Name)
	templateID := strings.TrimSpace(input.TemplateID)

	if name == "" || templateID == "" {
		return nil, ErrInvalidInput
	}

	if s.templateRepo != nil {
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
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	now := time.Now().UTC()
	rule := &domain.AutomationRule{
		ID:         uuid.NewString(),
		Name:       name,
		TemplateID: templateID,
		Enabled:    enabled,
		Config:     input.Config,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := rule.Validate(ctx); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, rule); err != nil {
		return nil, err
	}

	return rule, nil
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*domain.AutomationRule, error) {
	if s.repo == nil {
		return nil, errors.New("automation rule repository is nil")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
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
		if templateID != existing.TemplateID && s.templateRepo != nil {
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
		}
		existing.TemplateID = templateID
	}

	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}

	if input.Config != nil {
		existing.Config = *input.Config
	}

	existing.UpdatedAt = time.Now().UTC()

	if err := existing.Validate(ctx); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if s.repo == nil {
		return errors.New("automation rule repository is nil")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidInput
	}

	return s.repo.Delete(ctx, id)
}
