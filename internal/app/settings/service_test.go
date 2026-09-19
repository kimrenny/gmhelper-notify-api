package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
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
	svc := NewService(repo, nil)

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

	svc := NewService(repo, nil)
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
	svc := NewService(repo, nil)

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

	svc := NewService(repo, nil)
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
			svc := NewService(repo, nil)

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
	svc := NewService(repo, nil)

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

type mockActivityLogRepoForSettings struct {
	recordedLogs []*domain.ActivityLog
	createErr    error
}

func (m *mockActivityLogRepoForSettings) Create(ctx context.Context, log *domain.ActivityLog) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.recordedLogs = append(m.recordedLogs, log)
	return nil
}

func (m *mockActivityLogRepoForSettings) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepoForSettings) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func authContext(userID, role string) context.Context {
	p := &domain.Principal{UserID: userID, Role: role}
	return middleware.ContextWithPrincipal(context.Background(), p)
}

func TestSettingsService_Audit_SuccessfulUpdate(t *testing.T) {
	repo := newMockRepo()
	repo.settings[KeyDefaultFromName] = &domain.AppSetting{Key: KeyDefaultFromName, Value: "Old Name", Category: CategoryNotification}
	repo.settings[KeyReplyToEmail] = &domain.AppSetting{Key: KeyReplyToEmail, Value: "old@example.com", Category: CategoryNotification}
	repo.settings[KeyDefaultLocale] = &domain.AppSetting{Key: KeyDefaultLocale, Value: "en", Category: CategoryNotification}

	auditRepo := &mockActivityLogRepoForSettings{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("New Name"),
		ReplyToEmail:    stringPtr("new@example.com"),
		DefaultLocale:   stringPtr("ua"),
	}

	updated, err := svc.UpdateSettings(ctx, input)
	if err != nil {
		t.Fatalf("expected update success, got: %v", err)
	}

	if updated.DefaultFromName != "New Name" || updated.ReplyToEmail != "new@example.com" || updated.DefaultLocale != "ua" {
		t.Errorf("unexpected updated values: %+v", updated)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventSettingsUpdated {
		t.Errorf("expected EventType %s, got %s", domain.EventSettingsUpdated, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser || log.ActorUserID == nil || *log.ActorUserID != "usr-owner-1" {
		t.Errorf("unexpected actor: %+v", log)
	}
	if log.TargetType != domain.TargetTypeSettings || log.TargetID != "notification" {
		t.Errorf("expected target settings with ID 'notification', got type=%s id=%s", log.TargetType, log.TargetID)
	}
	if log.Summary != "Updated notification settings" {
		t.Errorf("expected summary 'Updated notification settings', got '%s'", log.Summary)
	}

	var details map[string]map[string]string
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}

	if details["before"]["defaultFromName"] != "Old Name" || details["after"]["defaultFromName"] != "New Name" {
		t.Errorf("unexpected from name diff: %+v", details)
	}
	if details["before"]["replyToEmail"] != "old@example.com" || details["after"]["replyToEmail"] != "new@example.com" {
		t.Errorf("unexpected reply-to diff: %+v", details)
	}
	if details["before"]["defaultLocale"] != "en" || details["after"]["defaultLocale"] != "ua" {
		t.Errorf("unexpected locale diff: %+v", details)
	}
}

func TestSettingsService_Audit_NoOpUpdate_NoAudit(t *testing.T) {
	repo := newMockRepo()
	repo.settings[KeyDefaultFromName] = &domain.AppSetting{Key: KeyDefaultFromName, Value: "Same Name", Category: CategoryNotification}
	repo.settings[KeyReplyToEmail] = &domain.AppSetting{Key: KeyReplyToEmail, Value: "same@example.com", Category: CategoryNotification}
	repo.settings[KeyDefaultLocale] = &domain.AppSetting{Key: KeyDefaultLocale, Value: "en", Category: CategoryNotification}

	auditRepo := &mockActivityLogRepoForSettings{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("Same Name"),
		ReplyToEmail:    stringPtr("same@example.com"),
		DefaultLocale:   stringPtr("en"),
	}

	_, err := svc.UpdateSettings(ctx, input)
	if err != nil {
		t.Fatalf("expected success on no-op update, got: %v", err)
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs for no-op update, got %d", len(auditRepo.recordedLogs))
	}
}

func TestSettingsService_Audit_ValidationFailure_NoAudit(t *testing.T) {
	repo := newMockRepo()
	auditRepo := &mockActivityLogRepoForSettings{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := UpdateAppSettingsInput{
		ReplyToEmail: stringPtr("invalid-email-address"),
	}

	_, err := svc.UpdateSettings(ctx, input)
	if err == nil {
		t.Fatal("expected error on invalid email, got nil")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on validation failure, got %d", len(auditRepo.recordedLogs))
	}
}

func TestSettingsService_Audit_SecretsExcluded(t *testing.T) {
	repo := newMockRepo()
	auditRepo := &mockActivityLogRepoForSettings{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("Safe Name"),
		DefaultLocale:   stringPtr("de"),
	}

	_, err := svc.UpdateSettings(ctx, input)
	if err != nil {
		t.Fatalf("expected update success, got: %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.recordedLogs))
	}

	detailsJSON := string(auditRepo.recordedLogs[0].Details)
	forbiddenWords := []string{"password", "secret", "smtp", "token", "jwt", "database", "postgres"}
	for _, word := range forbiddenWords {
		if strings.Contains(strings.ToLower(detailsJSON), word) {
			t.Errorf("audit details contained forbidden secret or infrastructure keyword '%s': %s", word, detailsJSON)
		}
	}
}

func TestSettingsService_Audit_AuditFailure_PropagatesError(t *testing.T) {
	repo := newMockRepo()
	auditRepo := &mockActivityLogRepoForSettings{
		createErr: errors.New("audit log write failure"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	ctx := authContext("usr-owner-1", "Owner")
	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("New Name"),
	}

	_, err := svc.UpdateSettings(ctx, input)
	if err == nil {
		t.Fatal("expected audit failure to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "audit log write failure") {
		t.Errorf("expected audit failure message, got: %v", err)
	}
}

func TestSettingsService_Audit_MissingPrincipal_ReturnsError(t *testing.T) {
	repo := newMockRepo()
	auditRepo := &mockActivityLogRepoForSettings{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(repo, auditSvc)

	// Unauthenticated context
	input := UpdateAppSettingsInput{
		DefaultFromName: stringPtr("New Name"),
	}

	_, err := svc.UpdateSettings(context.Background(), input)
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for unauthenticated context, got: %v", err)
	}
}
