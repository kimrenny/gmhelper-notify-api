package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
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
	execRepo     domain.AutomationExecutionRepository
	audit        *audit.Service
}

func NewService(repo domain.AutomationRuleRepository, templateRepo domain.EmailTemplateRepository, execRepo domain.AutomationExecutionRepository, audit *audit.Service) *Service {
	return &Service{
		repo:         repo,
		templateRepo: templateRepo,
		execRepo:     execRepo,
		audit:        audit,
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
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrInvalidInput
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListExecutions(ctx context.Context, ruleID string, limit, offset int) ([]*domain.AutomationExecution, int, error) {
	if s.repo == nil {
		return nil, 0, errors.New("automation rule repository is nil")
	}
	if s.execRepo == nil {
		return nil, 0, errors.New("automation execution repository is nil")
	}

	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil, 0, ErrInvalidInput
	}
	if _, err := uuid.Parse(ruleID); err != nil {
		return nil, 0, ErrInvalidInput
	}

	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, 0, err
	}
	if rule == nil {
		return nil, 0, ErrNotFound
	}

	return s.execRepo.ListByRuleID(ctx, ruleID, limit, offset)
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

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}
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
	var nextEval *time.Time
	if enabled && strings.EqualFold(strings.TrimSpace(input.Config.Trigger), domain.TriggerUserInactive) {
		nextEval = NextEvaluationTime(input.Config.Schedule, nil, now, now)
	}

	rule := &domain.AutomationRule{
		ID:               uuid.NewString(),
		Name:             name,
		TemplateID:       templateID,
		Enabled:          enabled,
		Config:           input.Config,
		NextEvaluationAt: nextEval,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := rule.Validate(ctx); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, rule); err != nil {
		return nil, err
	}

	if s.audit != nil {
		targetName := rule.Name
		_, err := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventAutomationCreated,
			Actor:      actor,
			TargetType: domain.TargetTypeAutomationRule,
			TargetID:   rule.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    fmt.Sprintf("Created automation rule %q", rule.Name),
			Details:    ruleSnapshot(rule),
		})
		if err != nil {
			return nil, err
		}
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

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}

	before := cloneRule(existing)
	target := cloneRule(existing)

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrInvalidInput
		}
		target.Name = name
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
		target.TemplateID = templateID
	}

	if input.Enabled != nil {
		target.Enabled = *input.Enabled
	}

	if input.Config != nil {
		target.Config = *input.Config
	}

	target.UpdatedAt = time.Now().UTC()
	if target.Enabled && strings.EqualFold(strings.TrimSpace(target.Config.Trigger), domain.TriggerUserInactive) {
		target.NextEvaluationAt = NextEvaluationTime(target.Config.Schedule, target.LastEvaluatedAt, target.CreatedAt, target.UpdatedAt)
	} else {
		target.NextEvaluationAt = nil
		if !strings.EqualFold(strings.TrimSpace(target.Config.Trigger), domain.TriggerUserInactive) {
			target.LastEvaluatedAt = nil
		}
	}

	if err := target.Validate(ctx); err != nil {
		return nil, err
	}

	// Change detection / No-Op check
	isChanged := before.Name != target.Name ||
		before.TemplateID != target.TemplateID ||
		before.Enabled != target.Enabled ||
		!configsEqual(before.Config, target.Config)

	if !isChanged {
		return existing, nil
	}

	if err := s.repo.Update(ctx, target); err != nil {
		return nil, err
	}

	if s.audit != nil {
		targetName := target.Name
		onlyEnabledChanged := (before.Name == target.Name) &&
			(before.TemplateID == target.TemplateID) &&
			configsEqual(before.Config, target.Config) &&
			(before.Enabled != target.Enabled)

		var eventType string
		var summary string
		var details any

		if onlyEnabledChanged {
			if target.Enabled {
				eventType = domain.EventAutomationEnabled
				summary = fmt.Sprintf("Enabled automation rule %q", target.Name)
			} else {
				eventType = domain.EventAutomationDisabled
				summary = fmt.Sprintf("Disabled automation rule %q", target.Name)
			}
			details = map[string]any{
				"ruleId":           target.ID,
				"ruleName":         target.Name,
				"previousEnabled":  before.Enabled,
				"resultingEnabled": target.Enabled,
			}
		} else {
			eventType = domain.EventAutomationUpdated
			summary = fmt.Sprintf("Updated automation rule %q", target.Name)
			details = map[string]any{
				"before": ruleSnapshot(before),
				"after":  ruleSnapshot(target),
			}
		}

		_, err = s.audit.Record(ctx, audit.RecordInput{
			EventType:  eventType,
			Actor:      actor,
			TargetType: domain.TargetTypeAutomationRule,
			TargetID:   target.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details:    details,
		})
		if err != nil {
			return nil, err
		}
	}

	return target, nil
}

func (s *Service) Enable(ctx context.Context, id string) (*domain.AutomationRule, error) {
	enabled := true
	return s.Update(ctx, id, UpdateInput{
		Enabled: &enabled,
	})
}

func (s *Service) Disable(ctx context.Context, id string) (*domain.AutomationRule, error) {
	enabled := false
	return s.Update(ctx, id, UpdateInput{
		Enabled: &enabled,
	})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if s.repo == nil {
		return errors.New("automation rule repository is nil")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidInput
	}

	var actor audit.Actor
	if s.audit != nil {
		var err error
		actor, err = audit.ActorFromContext(ctx, nil)
		if err != nil {
			return err
		}
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return domain.ErrNotFound
	}

	snapshot := ruleSnapshot(existing)

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	if s.audit != nil {
		targetName := existing.Name
		_, err := s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventAutomationDeleted,
			Actor:      actor,
			TargetType: domain.TargetTypeAutomationRule,
			TargetID:   existing.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    fmt.Sprintf("Deleted automation rule %q", existing.Name),
			Details:    snapshot,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func ruleSnapshot(r *domain.AutomationRule) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"id":         r.ID,
		"name":       r.Name,
		"templateId": r.TemplateID,
		"enabled":    r.Enabled,
		"config":     r.Config,
	}
}

func cloneRule(r *domain.AutomationRule) *domain.AutomationRule {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

func configsEqual(c1, c2 domain.AutomationRuleConfig) bool {
	b1, err1 := json.Marshal(c1)
	b2, err2 := json.Marshal(c2)
	if err1 == nil && err2 == nil {
		return bytes.Equal(b1, b2)
	}
	return reflect.DeepEqual(c1, c2)
}
