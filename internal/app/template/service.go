package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput    = errors.New("invalid template input")
	ErrNotFound        = domain.ErrNotFound
	ErrConflict        = domain.ErrConflict
	ErrMissingVariable = direct.ErrMissingVariable
)

type RenderedTemplate struct {
	Subject       string `json:"subject"`
	HTMLBody      string `json:"htmlBody"`
	PlainTextBody string `json:"plainTextBody,omitempty"`
}

type CreateInput struct {
	TemplateKey   string
	Name          string
	TemplateType  string
	Subject       string
	HTMLBody      string
	PlainTextBody string
	Locale        string
	Status        string
	Version       int
}

type UpdateInput struct {
	TemplateKey   string
	Name          string
	TemplateType  string
	Subject       string
	HTMLBody      string
	PlainTextBody string
	Locale        string
	Status        string
	Version       int
}

type Service struct {
	repo  domain.EmailTemplateRepository
	audit *audit.Service
}

func NewService(repo domain.EmailTemplateRepository, audit *audit.Service) *Service {
	return &Service{
		repo:  repo,
		audit: audit,
	}
}

func (s *Service) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.EmailTemplate, error) {
	templateKey := strings.TrimSpace(input.TemplateKey)
	name := strings.TrimSpace(input.Name)
	templateTypeStr := strings.TrimSpace(input.TemplateType)
	subject := strings.TrimSpace(input.Subject)
	htmlBody := strings.TrimSpace(input.HTMLBody)
	locale := strings.TrimSpace(input.Locale)
	statusStr := strings.TrimSpace(input.Status)

	if templateKey == "" || name == "" || templateTypeStr == "" || subject == "" || htmlBody == "" {
		return nil, ErrInvalidInput
	}

	templateType := domain.TemplateType(templateTypeStr)
	if !templateType.IsValid() {
		return nil, ErrInvalidInput
	}

	if locale == "" {
		locale = "en"
	}

	status := domain.TemplateStatusActive
	if statusStr != "" {
		status = domain.TemplateStatus(statusStr)
		if !status.IsValid() {
			return nil, ErrInvalidInput
		}
	}

	version := input.Version
	if version <= 0 {
		version = 1
	}

	now := time.Now().UTC()
	template := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   templateKey,
		Name:          name,
		TemplateType:  templateType,
		Subject:       subject,
		HTMLBody:      htmlBody,
		PlainTextBody: input.PlainTextBody,
		Locale:        locale,
		Status:        status,
		Version:       version,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(ctx, template); err != nil {
		return nil, err
	}

	if s.audit != nil {
		actor, err := audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}

		details := map[string]any{
			"templateKey":  template.TemplateKey,
			"name":         template.Name,
			"templateType": string(template.TemplateType),
			"subject":      template.Subject,
			"htmlBody":     template.HTMLBody,
			"locale":       template.Locale,
			"status":       string(template.Status),
			"version":      template.Version,
		}
		if template.PlainTextBody != "" {
			details["plainTextBody"] = template.PlainTextBody
		}

		targetName := template.Name
		_, err = s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventTemplateCreated,
			Actor:      actor,
			TargetType: domain.TargetTypeTemplate,
			TargetID:   template.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    fmt.Sprintf("Created email template %q", template.Name),
			Details:    details,
		})
		if err != nil {
			return nil, err
		}
	}

	return template, nil
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*domain.EmailTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	templateKey := strings.TrimSpace(input.TemplateKey)
	name := strings.TrimSpace(input.Name)
	subject := strings.TrimSpace(input.Subject)
	htmlBody := strings.TrimSpace(input.HTMLBody)
	locale := strings.TrimSpace(input.Locale)
	statusStr := strings.TrimSpace(input.Status)

	if templateKey == "" || name == "" || subject == "" || htmlBody == "" {
		return nil, ErrInvalidInput
	}

	if locale == "" {
		locale = "en"
	}

	status := domain.TemplateStatusActive
	if statusStr != "" {
		status = domain.TemplateStatus(statusStr)
		if !status.IsValid() {
			return nil, ErrInvalidInput
		}
	}

	version := input.Version
	if version <= 0 {
		version = 1
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Capture before snapshot
	beforeDetails := map[string]any{
		"templateKey":  existing.TemplateKey,
		"name":         existing.Name,
		"templateType": string(existing.TemplateType),
		"subject":      existing.Subject,
		"htmlBody":     existing.HTMLBody,
		"locale":       existing.Locale,
		"status":       string(existing.Status),
		"version":      existing.Version,
	}
	if existing.PlainTextBody != "" {
		beforeDetails["plainTextBody"] = existing.PlainTextBody
	}
	beforeStatus := existing.Status
	beforeSubject := existing.Subject
	beforeHTMLBody := existing.HTMLBody
	beforePlainText := existing.PlainTextBody
	beforeLocale := existing.Locale
	beforeName := existing.Name
	beforeKey := existing.TemplateKey
	beforeVersion := existing.Version

	// TemplateType is immutable. If provided, validate that it matches existing type.
	tTypeStr := strings.TrimSpace(input.TemplateType)
	if tTypeStr != "" {
		tType := domain.TemplateType(tTypeStr)
		if !tType.IsValid() || tType != existing.TemplateType {
			return nil, ErrInvalidInput
		}
	}

	existing.TemplateKey = templateKey
	existing.Name = name
	existing.Subject = subject
	existing.HTMLBody = htmlBody
	existing.PlainTextBody = input.PlainTextBody
	existing.Locale = locale
	existing.Status = status
	existing.Version = version
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	if s.audit != nil {
		actor, err := audit.ActorFromContext(ctx, nil)
		if err != nil {
			return nil, err
		}

		afterDetails := map[string]any{
			"templateKey":  existing.TemplateKey,
			"name":         existing.Name,
			"templateType": string(existing.TemplateType),
			"subject":      existing.Subject,
			"htmlBody":     existing.HTMLBody,
			"locale":       existing.Locale,
			"status":       string(existing.Status),
			"version":      existing.Version,
		}
		if existing.PlainTextBody != "" {
			afterDetails["plainTextBody"] = existing.PlainTextBody
		}

		targetName := existing.Name
		var eventType string
		var summary string
		var details any

		// If only status changed and content fields remain unchanged
		if beforeStatus != existing.Status &&
			beforeSubject == existing.Subject &&
			beforeHTMLBody == existing.HTMLBody &&
			beforePlainText == existing.PlainTextBody &&
			beforeLocale == existing.Locale &&
			beforeName == existing.Name &&
			beforeKey == existing.TemplateKey &&
			beforeVersion == existing.Version {
			if existing.Status == domain.TemplateStatusArchived {
				eventType = domain.EventTemplateArchived
				summary = fmt.Sprintf("Archived email template %q", existing.Name)
			} else {
				eventType = domain.EventTemplateStatusChanged
				summary = fmt.Sprintf("Changed status of email template %q to %s", existing.Name, existing.Status)
			}
			details = map[string]any{
				"before": map[string]any{"status": string(beforeStatus)},
				"after":  map[string]any{"status": string(existing.Status)},
			}
		} else {
			eventType = domain.EventTemplateUpdated
			summary = fmt.Sprintf("Updated email template %q", existing.Name)
			details = map[string]any{
				"before": beforeDetails,
				"after":  afterDetails,
			}
		}

		_, err = s.audit.Record(ctx, audit.RecordInput{
			EventType:  eventType,
			Actor:      actor,
			TargetType: domain.TargetTypeTemplate,
			TargetID:   existing.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    summary,
			Details:    details,
		})
		if err != nil {
			return nil, err
		}
	}

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidInput
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	if s.audit != nil {
		actor, err := audit.ActorFromContext(ctx, nil)
		if err != nil {
			return err
		}

		targetName := existing.Name
		_, err = s.audit.Record(ctx, audit.RecordInput{
			EventType:  domain.EventTemplateArchived,
			Actor:      actor,
			TargetType: domain.TargetTypeTemplate,
			TargetID:   existing.ID,
			TargetName: &targetName,
			Status:     domain.ActivityStatusSuccess,
			Summary:    fmt.Sprintf("Archived email template %q", existing.Name),
			Details: map[string]any{
				"before": map[string]any{
					"status": string(existing.Status),
				},
				"after": map[string]any{
					"status": "archived",
				},
				"templateKey":  existing.TemplateKey,
				"name":         existing.Name,
				"templateType": string(existing.TemplateType),
				"locale":       existing.Locale,
				"version":      existing.Version,
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

type PreviewInput struct {
	Subject       *string
	HTMLBody      *string
	PlainTextBody *string
	Variables     map[string]any
}

func (s *Service) Preview(ctx context.Context, id string, input PreviewInput) (*RenderedTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrInvalidInput
	}

	var subject, htmlBody, plainTextBody string

	// Try loading from repository if template ID exists in database
	tpl, err := s.repo.GetByID(ctx, id)
	if err == nil {
		subject = tpl.Subject
		htmlBody = tpl.HTMLBody
		plainTextBody = tpl.PlainTextBody
		if input.Subject != nil {
			subject = *input.Subject
		}
		if input.HTMLBody != nil {
			htmlBody = *input.HTMLBody
		}
		if input.PlainTextBody != nil {
			plainTextBody = *input.PlainTextBody
		}
	} else {
		// If template is not in DB (e.g. unsaved/new template preview), require subject & htmlBody in overrides
		if input.Subject != nil && input.HTMLBody != nil {
			subject = *input.Subject
			htmlBody = *input.HTMLBody
			if input.PlainTextBody != nil {
				plainTextBody = *input.PlainTextBody
			}
		} else {
			return nil, err
		}
	}

	rendered, err := direct.RenderEmail(subject, htmlBody, plainTextBody, input.Variables)
	if err != nil {
		return nil, err
	}

	return &RenderedTemplate{
		Subject:       rendered.Subject,
		HTMLBody:      rendered.HTMLBody,
		PlainTextBody: rendered.PlainTextBody,
	}, nil
}
