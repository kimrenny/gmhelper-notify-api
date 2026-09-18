package settings

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockAppSettingRepo struct {
	settings map[string]*domain.AppSetting
	saveErr  error
	listErr  error
}

func newMockRepo() *mockAppSettingRepo {
	return &mockAppSettingRepo{
		settings: make(map[string]*domain.AppSetting),
	}
}

func (m *mockAppSettingRepo) GetByKey(ctx context.Context, key string) (*domain.AppSetting, error) {
	if s, ok := m.settings[key]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockAppSettingRepo) Save(ctx context.Context, setting *domain.AppSetting) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	now := time.Now().UTC()
	m.settings[setting.Key] = &domain.AppSetting{
		Key:         setting.Key,
		Value:       setting.Value,
		Category:    setting.Category,
		Description: setting.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return nil
}

func (m *mockAppSettingRepo) ListByCategory(ctx context.Context, category string) ([]*domain.AppSetting, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AppSetting
	for _, s := range m.settings {
		if s.Category == category {
			res = append(res, s)
		}
	}
	return res, nil
}

func (m *mockAppSettingRepo) ListAll(ctx context.Context) ([]*domain.AppSetting, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AppSetting
	for _, s := range m.settings {
		res = append(res, s)
	}
	return res, nil
}

func stringPtr(s string) *string {
	return &s
}

func TestService_GetSettings_DefaultsWhenEmpty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	dto, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error getting default settings: %v", err)
	}

	if dto.DefaultFromName != "" {
		t.Errorf("expected empty DefaultFromName, got '%s'", dto.DefaultFromName)
	}
	if dto.ReplyToEmail != "" {
		t.Errorf("expected empty ReplyToEmail, got '%s'", dto.ReplyToEmail)
	}
	if dto.DefaultLocale != "en" {
		t.Errorf("expected default locale 'en', got '%s'", dto.DefaultLocale)
	}
}

func TestService_GetSettings_WithPersistedValues(t *testing.T) {
	repo := newMockRepo()
	repo.settings[KeyDefaultFromName] = &domain.AppSetting{Key: KeyDefaultFromName, Value: "GMHelper Notifications", Category: CategoryNotification}
	repo.settings[KeyReplyToEmail] = &domain.AppSetting{Key: KeyReplyToEmail, Value: "support@gmhelper.com", Category: CategoryNotification}
	repo.settings[KeyDefaultLocale] = &domain.AppSetting{Key: KeyDefaultLocale, Value: "ua", Category: CategoryNotification}

	svc := NewService(repo)
	dto, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dto.DefaultFromName != "GMHelper Notifications" {
		t.Errorf("expected 'GMHelper Notifications', got '%s'", dto.DefaultFromName)
	}
	if dto.ReplyToEmail != "support@gmhelper.com" {
		t.Errorf("expected 'support@gmhelper.com', got '%s'", dto.ReplyToEmail)
	}
	if dto.DefaultLocale != "ua" {
		t.Errorf("expected 'ua', got '%s'", dto.DefaultLocale)
	}
}

func TestService_UpdateSettings_ValidAll(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("GMHelper Alerts"),
		ReplyToEmail:    stringPtr("replies@gmhelper.com"),
		DefaultLocale:   stringPtr("de"),
	}

	res, err := svc.UpdateSettings(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error updating settings: %v", err)
	}

	if res.DefaultFromName != "GMHelper Alerts" {
		t.Errorf("expected DefaultFromName 'GMHelper Alerts', got '%s'", res.DefaultFromName)
	}
	if res.ReplyToEmail != "replies@gmhelper.com" {
		t.Errorf("expected ReplyToEmail 'replies@gmhelper.com', got '%s'", res.ReplyToEmail)
	}
	if res.DefaultLocale != "de" {
		t.Errorf("expected DefaultLocale 'de', got '%s'", res.DefaultLocale)
	}

	// Verify in repository
	if repo.settings[KeyDefaultFromName].Value != "GMHelper Alerts" {
		t.Errorf("repo mismatch for from name")
	}
	if repo.settings[KeyReplyToEmail].Value != "replies@gmhelper.com" {
		t.Errorf("repo mismatch for reply to")
	}
	if repo.settings[KeyDefaultLocale].Value != "de" {
		t.Errorf("repo mismatch for default locale")
	}
}

func TestService_UpdateSettings_EmptyReplyToAllowed(t *testing.T) {
	repo := newMockRepo()
	repo.settings[KeyReplyToEmail] = &domain.AppSetting{Key: KeyReplyToEmail, Value: "old@example.com", Category: CategoryNotification}

	svc := NewService(repo)
	input := UpdateAppSettingsInput{
		ReplyToEmail: stringPtr(""),
	}

	res, err := svc.UpdateSettings(context.Background(), input)
	if err != nil {
		t.Fatalf("expected clearing reply-to to succeed: %v", err)
	}

	if res.ReplyToEmail != "" {
		t.Errorf("expected empty reply-to email, got '%s'", res.ReplyToEmail)
	}
	if repo.settings[KeyReplyToEmail].Value != "" {
		t.Errorf("expected repository value to be empty string")
	}
}

func TestService_UpdateSettings_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       UpdateAppSettingsInput
		errContains string
	}{
		{
			name: "From name exceeds max length",
			input: UpdateAppSettingsInput{
				DefaultFromName: stringPtr(strings.Repeat("a", MaxFromNameLength+1)),
			},
			errContains: "default from name exceeds maximum length",
		},
		{
			name: "Invalid email format missing domain",
			input: UpdateAppSettingsInput{
				ReplyToEmail: stringPtr("invalid-email-address"),
			},
			errContains: "invalid email",
		},
		{
			name: "Invalid email missing dot in domain",
			input: UpdateAppSettingsInput{
				ReplyToEmail: stringPtr("user@localdomain"),
			},
			errContains: "invalid email domain",
		},
		{
			name: "Email exceeds max length",
			input: UpdateAppSettingsInput{
				ReplyToEmail: stringPtr(strings.Repeat("a", 250) + "@domain.com"),
			},
			errContains: "reply-to email exceeds maximum length",
		},
		{
			name: "Empty default locale",
			input: UpdateAppSettingsInput{
				DefaultLocale: stringPtr(""),
			},
			errContains: "default locale cannot be empty",
		},
		{
			name: "Unsupported locale",
			input: UpdateAppSettingsInput{
				DefaultLocale: stringPtr("klingon"),
			},
			errContains: "unsupported locale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			svc := NewService(repo)

			_, err := svc.UpdateSettings(context.Background(), tt.input)
			if err == nil {
				t.Fatalf("expected error containing '%s', got nil", tt.errContains)
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error containing '%s', got '%s'", tt.errContains, err.Error())
			}
		})
	}
}

func TestService_UpdateSettings_RepositoryError(t *testing.T) {
	repo := newMockRepo()
	repo.saveErr = errors.New("db error on save")
	svc := NewService(repo)

	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("Valid Name"),
	}

	_, err := svc.UpdateSettings(context.Background(), input)
	if err == nil {
		t.Fatalf("expected db error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to save default from name setting") {
		t.Errorf("unexpected error message: %v", err)
	}
}
