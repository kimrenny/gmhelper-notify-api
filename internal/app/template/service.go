package template

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid template input")
	ErrNotFound     = domain.ErrNotFound
	ErrConflict     = domain.ErrConflict
)

type CreateInput struct {
	TemplateKey   string
	Name          string
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
	Subject       string
	HTMLBody      string
	PlainTextBody string
	Locale        string
	Status        string
	Version       int
}

type Service struct {
	repo domain.EmailTemplateRepository
}

func NewService(repo domain.EmailTemplateRepository) *Service {
	return &Service{repo: repo}
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

	now := time.Now().UTC()
	template := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   templateKey,
		Name:          name,
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

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidInput
	}
	return s.repo.Delete(ctx, id)
}
