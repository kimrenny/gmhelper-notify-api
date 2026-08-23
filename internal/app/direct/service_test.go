package direct

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *mockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}

func (m *mockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *mockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *mockTemplateRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type mockDirectRepo struct {
	notifications map[string]*domain.DirectNotification
	attempts      map[string]*domain.DeliveryAttempt
	createErr     error
}

func (m *mockDirectRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	n, ok := m.notifications[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (m *mockDirectRepo) Create(ctx context.Context, notification *domain.DirectNotification) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications[notification.ID] = notification
	return nil
}

func (m *mockDirectRepo) CreateWithInitialAttempt(ctx context.Context, notification *domain.DirectNotification, attempt *domain.DeliveryAttempt) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications[notification.ID] = notification
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockDirectRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	var list []*domain.DirectNotification
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending {
			list = append(list, n)
		}
	}
	return list, nil
}

func (m *mockDirectRepo) ClaimPending(ctx context.Context, limit int) ([]*domain.DirectNotification, error) {
	var list []*domain.DirectNotification
	now := time.Now().UTC()
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending {
			n.DeliveryStatus = domain.DeliveryStatusSending
			n.AttemptsCount++
			n.LastAttemptAt = &now
			list = append(list, n)
			if limit > 0 && len(list) >= limit {
				break
			}
		}
	}
	return list, nil
}

func (m *mockDirectRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration) (int64, error) {
	var count int64
	cutoff := time.Now().UTC().Add(-olderThan)
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusSending && n.LastAttemptAt != nil && n.LastAttemptAt.Before(cutoff) {
			n.DeliveryStatus = domain.DeliveryStatusPending
			n.ErrorMessage = "delivery claim timed out and was recovered"
			count++
		}
	}
	return count, nil
}

func (m *mockDirectRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	n, ok := m.notifications[id]
	if !ok {
		return domain.ErrNotFound
	}
	n.DeliveryStatus = status
	n.AttemptsCount = attempts
	n.LastAttemptAt = lastAttemptAt
	n.SentAt = sentAt
	n.ErrorMessage = errorMessage
	return nil
}

func setupTestService() (*Service, *mockTemplateRepo, *mockDirectRepo) {
	tplRepo := &mockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	svc := NewService(tplRepo, directRepo)
	return svc, tplRepo, directRepo
}

func TestService_CreateDirectNotification_Success(t *testing.T) {
	svc, tplRepo, directRepo := setupTestService()

	activeTpl := &domain.EmailTemplate{
		ID:            "tpl-active-1",
		TemplateKey:   "welcome_user",
		Name:          "Welcome User",
		Subject:       "Welcome to GMHelper, {{userName}}!",
		HTMLBody:      "<p>Hi {{userName}}, your activation code is {{code}}.</p>",
		PlainTextBody: "Hi {{userName}}, your activation code is {{code}}.",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:       activeTpl.ID,
		ExternalUserID:   "user-100",
		RecipientEmail:   "alice@example.com",
		RecipientName:    "Alice",
		NotificationType: domain.NotificationTypeDirect,
		Payload: map[string]any{
			"userName": "Alice",
			"code":     "XYZ-999",
		},
	}

	result, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected Create success, got error: %v", err)
	}

	if result.Notification == nil || result.Notification.ID == "" {
		t.Fatal("expected non-empty notification ID")
	}
	if result.Notification.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected pending delivery status, got %s", result.Notification.DeliveryStatus)
	}
	if result.Notification.RecipientEmail != "alice@example.com" {
		t.Errorf("expected recipient email alice@example.com, got %s", result.Notification.RecipientEmail)
	}

	// Verify atomic persistence of initial attempt
	if len(directRepo.attempts) != 1 {
		t.Fatalf("expected 1 persisted delivery attempt, got %d", len(directRepo.attempts))
	}
	for _, attempt := range directRepo.attempts {
		if attempt.TargetID != result.Notification.ID {
			t.Errorf("expected attempt target ID %s, got %s", result.Notification.ID, attempt.TargetID)
		}
		if attempt.AttemptNumber != 1 {
			t.Errorf("expected attempt number 1, got %d", attempt.AttemptNumber)
		}
		if attempt.Status != domain.DeliveryStatusPending {
			t.Errorf("expected attempt status pending, got %s", attempt.Status)
		}
	}

	// Verify rendered email output
	if result.Rendered.Subject != "Welcome to GMHelper, Alice!" {
		t.Errorf("unexpected rendered subject: %s", result.Rendered.Subject)
	}
	if result.Rendered.HTMLBody != "<p>Hi Alice, your activation code is XYZ-999.</p>" {
		t.Errorf("unexpected rendered html body: %s", result.Rendered.HTMLBody)
	}
}

func TestService_CreateDirectNotification_TemplateNotFound(t *testing.T) {
	svc, _, _ := setupTestService()

	input := CreateInput{
		TemplateID:     "missing-template-id",
		RecipientEmail: "alice@example.com",
	}

	_, err := svc.Create(context.Background(), input)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_CreateDirectNotification_TemplateInactive(t *testing.T) {
	svc, tplRepo, _ := setupTestService()

	draftTpl := &domain.EmailTemplate{
		ID:          "tpl-draft-1",
		TemplateKey: "draft_key",
		Name:        "Draft Template",
		Subject:     "Draft Subject",
		HTMLBody:    "<p>Draft Body</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusDraft,
		Version:     1,
	}
	tplRepo.templates[draftTpl.ID] = draftTpl

	input := CreateInput{
		TemplateID:     draftTpl.ID,
		RecipientEmail: "alice@example.com",
	}

	_, err := svc.Create(context.Background(), input)
	if !errors.Is(err, ErrTemplateInactive) {
		t.Fatalf("expected ErrTemplateInactive, got %v", err)
	}
}

func TestService_CreateDirectNotification_InvalidEmail(t *testing.T) {
	svc, tplRepo, _ := setupTestService()

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-1",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	invalidEmails := []string{
		"",
		"invalid-email",
		"alice@",
		"@example.com",
		"alice@example",
	}

	for _, email := range invalidEmails {
		input := CreateInput{
			TemplateID:     activeTpl.ID,
			RecipientEmail: email,
		}
		_, err := svc.Create(context.Background(), input)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for email '%s', got %v", email, err)
		}
	}
}

func TestService_CreateDirectNotification_MissingTemplateVariable(t *testing.T) {
	svc, tplRepo, _ := setupTestService()

	activeTpl := &domain.EmailTemplate{
		ID:          "tpl-1",
		TemplateKey: "security_alert",
		Name:        "Security Alert",
		Subject:     "Security Alert for {{user}}",
		HTMLBody:    "<p>Device: {{device}}, Location: {{location}}</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		RecipientEmail: "user@example.com",
		Payload: map[string]any{
			"user":   "Alice",
			"device": "iPhone",
			// location is missing
		},
	}

	_, err := svc.Create(context.Background(), input)
	if !errors.Is(err, ErrMissingVariable) {
		t.Fatalf("expected ErrMissingVariable, got %v", err)
	}
}

func TestService_CreateDirectNotification_RepoFailure(t *testing.T) {
	svc, tplRepo, directRepo := setupTestService()

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-1",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl
	directRepo.createErr = errors.New("database connection failed")

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		RecipientEmail: "user@example.com",
	}

	_, err := svc.Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error on repository failure, got nil")
	}
}

func TestService_GetByID_And_ListPending(t *testing.T) {
	svc, _, directRepo := setupTestService()

	n1 := &domain.DirectNotification{
		ID:             "notif-1",
		RecipientEmail: "a@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	directRepo.notifications[n1.ID] = n1

	// 1. GetByID Success
	found, err := svc.GetByID(context.Background(), "notif-1")
	if err != nil {
		t.Fatalf("expected found notification, got error: %v", err)
	}
	if found.ID != "notif-1" {
		t.Errorf("expected ID notif-1, got %s", found.ID)
	}

	// 2. GetByID Missing
	_, err = svc.GetByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. GetByID Empty ID
	_, err = svc.GetByID(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	// 4. ListPending
	pending, err := svc.ListPending(context.Background())
	if err != nil {
		t.Fatalf("expected ListPending success, got %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending notification, got %d", len(pending))
	}
}
