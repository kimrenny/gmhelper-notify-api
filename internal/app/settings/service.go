package settings

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/gmhelper/notify-api/internal/domain"
)

var (
	ErrInvalidInput = errors.New("invalid settings input")
)

const (
	CategoryNotification = "notification"

	KeyDefaultFromName = "notification.default_from_name"
	KeyReplyToEmail    = "notification.reply_to_email"
	KeyDefaultLocale   = "notification.default_locale"

	DefaultLocaleFallback = "en"

	MaxFromNameLength = 100
	MaxEmailLength    = 255
	MaxLocaleLength   = 35
)

// SupportedLocales contains the set of valid application locales.
var SupportedLocales = map[string]bool{
	"en":    true,
	"en-us": true,
	"en-gb": true,
	"ua":    true,
	"uk":    true,
	"ru":    true,
	"de":    true,
	"es":    true,
	"fr":    true,
	"pl":    true,
	"it":    true,
}

// AppSettingsDTO represents safe, administrator-configurable application preferences.
type AppSettingsDTO struct {
	DefaultFromName string `json:"defaultFromName"`
	ReplyToEmail    string `json:"replyToEmail"`
	DefaultLocale   string `json:"defaultLocale"`
}

// UpdateAppSettingsInput defines the payload for updating application preferences.
type UpdateAppSettingsInput struct {
	DefaultFromName *string `json:"defaultFromName,omitempty"`
	ReplyToEmail    *string `json:"replyToEmail,omitempty"`
	DefaultLocale   *string `json:"defaultLocale,omitempty"`
}

// Service manages retrieving and persisting application preferences.
type Service struct {
	repo domain.AppSettingRepository
}

// NewService constructs a new settings Service.
func NewService(repo domain.AppSettingRepository) *Service {
	return &Service{repo: repo}
}

// GetSettings retrieves the current safe application preferences with clean default fallbacks.
func (s *Service) GetSettings(ctx context.Context) (*AppSettingsDTO, error) {
	dto := &AppSettingsDTO{
		DefaultFromName: "",
		ReplyToEmail:    "",
		DefaultLocale:   DefaultLocaleFallback,
	}

	settings, err := s.repo.ListByCategory(ctx, CategoryNotification)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("failed to list settings: %w", err)
	}

	for _, item := range settings {
		switch item.Key {
		case KeyDefaultFromName:
			dto.DefaultFromName = item.Value
		case KeyReplyToEmail:
			dto.ReplyToEmail = item.Value
		case KeyDefaultLocale:
			if strings.TrimSpace(item.Value) != "" {
				dto.DefaultLocale = item.Value
			}
		}
	}

	return dto, nil
}

// UpdateSettings validates and updates the specified application preferences.
func (s *Service) UpdateSettings(ctx context.Context, input UpdateAppSettingsInput) (*AppSettingsDTO, error) {
	current, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	if input.DefaultFromName != nil {
		name := strings.TrimSpace(*input.DefaultFromName)
		if utf8.RuneCountInString(name) > MaxFromNameLength {
			return nil, fmt.Errorf("%w: default from name exceeds maximum length of %d characters", ErrInvalidInput, MaxFromNameLength)
		}
		current.DefaultFromName = name
		if err := s.repo.Save(ctx, &domain.AppSetting{
			Key:         KeyDefaultFromName,
			Value:       name,
			Category:    CategoryNotification,
			Description: "Default display sender name for outgoing notifications",
		}); err != nil {
			return nil, fmt.Errorf("failed to save default from name setting: %w", err)
		}
	}

	if input.ReplyToEmail != nil {
		emailAddr := strings.TrimSpace(*input.ReplyToEmail)
		if emailAddr != "" {
			if len(emailAddr) > MaxEmailLength {
				return nil, fmt.Errorf("%w: reply-to email exceeds maximum length of %d characters", ErrInvalidInput, MaxEmailLength)
			}
			if err := validateEmail(emailAddr); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
			}
		}
		current.ReplyToEmail = emailAddr
		if err := s.repo.Save(ctx, &domain.AppSetting{
			Key:         KeyReplyToEmail,
			Value:       emailAddr,
			Category:    CategoryNotification,
			Description: "Optional reply-to email address for outgoing notifications",
		}); err != nil {
			return nil, fmt.Errorf("failed to save reply-to email setting: %w", err)
		}
	}

	if input.DefaultLocale != nil {
		loc := strings.TrimSpace(*input.DefaultLocale)
		if loc == "" {
			return nil, fmt.Errorf("%w: default locale cannot be empty", ErrInvalidInput)
		}
		if len(loc) > MaxLocaleLength {
			return nil, fmt.Errorf("%w: default locale exceeds maximum length of %d characters", ErrInvalidInput, MaxLocaleLength)
		}
		normalizedLoc := strings.ToLower(loc)
		if !SupportedLocales[normalizedLoc] {
			return nil, fmt.Errorf("%w: unsupported locale '%s'", ErrInvalidInput, loc)
		}
		current.DefaultLocale = loc
		if err := s.repo.Save(ctx, &domain.AppSetting{
			Key:         KeyDefaultLocale,
			Value:       loc,
			Category:    CategoryNotification,
			Description: "Default fallback locale for notification templates and messaging",
		}); err != nil {
			return nil, fmt.Errorf("failed to save default locale setting: %w", err)
		}
	}

	return current, nil
}

func validateEmail(address string) error {
	addr, err := mail.ParseAddress(address)
	if err != nil || addr.Address != address {
		return fmt.Errorf("invalid email address format: '%s'", address)
	}
	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 || !strings.Contains(parts[1], ".") {
		return fmt.Errorf("invalid email domain: '%s'", address)
	}
	return nil
}
