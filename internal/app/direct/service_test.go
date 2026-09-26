package direct

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
	"github.com/gmhelper/notify-api/internal/infra/userclient"
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
	for _, t := range m.templates {
		if t.TemplateKey == templateKey {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) GetByKeyAndLocale(ctx context.Context, templateKey, locale string) (*domain.EmailTemplate, error) {
	for _, t := range m.templates {
		if t.TemplateKey == templateKey && (t.Locale == locale || locale == "") {
			return t, nil
		}
	}
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

func (m *mockDirectRepo) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	var list []*domain.DirectNotification
	now := time.Now().UTC()
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending && (maxAttempts <= 0 || n.AttemptsCount < maxAttempts) {
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

func (m *mockDirectRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	var count int64
	cutoff := time.Now().UTC().Add(-olderThan)
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusSending && n.LastAttemptAt != nil && n.LastAttemptAt.Before(cutoff) {
			if maxAttempts > 0 && n.AttemptsCount >= maxAttempts {
				n.DeliveryStatus = domain.DeliveryStatusFailed
				n.ErrorMessage = "delivery attempt timed out and max attempts reached"
			} else {
				n.DeliveryStatus = domain.DeliveryStatusPending
				n.ErrorMessage = "delivery claim timed out and was recovered"
			}
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

type mockUserResolver struct {
	getUserByIDFunc func(ctx context.Context, id string) (*userclient.User, error)
}

func (m *mockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	return &userclient.User{
		ID:       id,
		Username: "user_" + id,
		Email:    id + "@example.com",
	}, nil
}

func setupTestService(resolvers ...UserResolver) (*Service, *mockTemplateRepo, *mockDirectRepo) {
	tplRepo := &mockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	var resolver UserResolver = &mockUserResolver{}
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	svc := NewService(tplRepo, directRepo, resolver, nil)
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

func TestService_CreateDirectNotification_ExternalUser_Success(t *testing.T) {
	type ctxKey struct{}
	var passedID string
	var passedCtx context.Context

	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			passedID = id
			passedCtx = ctx
			return &userclient.User{
				ID:       id,
				Username: "bob_the_builder",
				Email:    "bob@example.com",
				Role:     "User",
				IsActive: true,
			}, nil
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-ext-1",
		Subject:  "Hello {{name}}",
		HTMLBody: "<p>Hello {{name}}</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	reqCtx := context.WithValue(context.Background(), ctxKey{}, "trace-999")
	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "user-ext-123",
		RecipientEmail: "bob@example.com",
		Payload: map[string]any{
			"name": "Bob",
		},
	}

	res, err := svc.Create(reqCtx, input)
	if err != nil {
		t.Fatalf("expected Create success with external user, got: %v", err)
	}

	if passedID != "user-ext-123" {
		t.Errorf("expected resolver called with user-ext-123, got: %s", passedID)
	}
	if passedCtx == nil || passedCtx.Value(ctxKey{}) != "trace-999" {
		t.Errorf("expected request context to be forwarded to user resolver")
	}
	if res.ResolvedUser == nil || res.ResolvedUser.Username != "bob_the_builder" {
		t.Errorf("expected ResolvedUser populated in CreateResult, got: %+v", res.ResolvedUser)
	}
	if res.Notification.ExternalUserID != "user-ext-123" {
		t.Errorf("expected notification ExternalUserID user-ext-123, got: %s", res.Notification.ExternalUserID)
	}
}

func TestService_CreateDirectNotification_ExternalUser_ErrorPropagation(t *testing.T) {
	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return nil, userclient.ErrNotFound
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-ext-err",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "non-existent-user",
		RecipientEmail: "user@example.com",
	}

	_, err := svc.Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when user resolution fails, got nil")
	}
	if !errors.Is(err, userclient.ErrNotFound) {
		t.Errorf("expected wrapped userclient.ErrNotFound, got: %v", err)
	}
}

func TestService_CreateDirectNotification_NoExternalUser_ResolverNotCalled(t *testing.T) {
	var resolverCalled bool
	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			resolverCalled = true
			return nil, nil
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-ext-no-call",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "", // empty
		RecipientEmail: "user@example.com",
	}

	res, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected Create success without external user, got: %v", err)
	}

	if resolverCalled {
		t.Error("expected resolver NOT to be called when ExternalUserID is empty")
	}
	if res.ResolvedUser != nil {
		t.Errorf("expected ResolvedUser to be nil when not provided, got: %+v", res.ResolvedUser)
	}
}

func TestService_CreateDirectNotification_ResolverUnavailable(t *testing.T) {
	// Service with nil userResolver
	svc, tplRepo, _ := setupTestService(nil)

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-nil-resolver",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "user-123",
		RecipientEmail: "user@example.com",
	}

	_, err := svc.Create(context.Background(), input)
	if !errors.Is(err, ErrUserResolverUnavailable) {
		t.Fatalf("expected ErrUserResolverUnavailable when resolver is nil and external user requested, got: %v", err)
	}
}

func TestService_CreateDirectNotification_UserVariables_Personalization(t *testing.T) {
	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return &userclient.User{
				ID:       id,
				Username: "janedoe",
				Email:    "jane@example.com",
				Role:     "Manager",
				Language: "fr",
			}, nil
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:            "tpl-user-vars",
		TemplateKey:   "account_summary",
		Subject:       "Summary for {{username}} (Role: {{role}})",
		HTMLBody:      "<p>Email: {{email}}, Lang: {{language}}</p>",
		PlainTextBody: "Email: {{email}}, Lang: {{language}}",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "user-456",
		RecipientEmail: "jane@example.com",
	}

	res, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected Create success with resolved user variables, got: %v", err)
	}

	expectedSubject := "Summary for janedoe (Role: Manager)"
	if res.Rendered.Subject != expectedSubject {
		t.Errorf("expected subject '%s', got '%s'", expectedSubject, res.Rendered.Subject)
	}

	expectedHTML := "<p>Email: jane@example.com, Lang: fr</p>"
	if res.Rendered.HTMLBody != expectedHTML {
		t.Errorf("expected HTML body '%s', got '%s'", expectedHTML, res.Rendered.HTMLBody)
	}

	expectedPlain := "Email: jane@example.com, Lang: fr"
	if res.Rendered.PlainTextBody != expectedPlain {
		t.Errorf("expected plain body '%s', got '%s'", expectedPlain, res.Rendered.PlainTextBody)
	}
}

func TestService_CreateDirectNotification_ExplicitVariablePrecedence(t *testing.T) {
	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return &userclient.User{
				ID:       id,
				Username: "johndoe",
				Email:    "john.original@example.com",
				Role:     "User",
				Language: "en",
			}, nil
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:            "tpl-precedence",
		TemplateKey:   "welcome_precedence",
		Subject:       "Hello {{username}}",
		HTMLBody:      "<p>User: {{username}}, Email: {{email}}, Extra: {{extra}}</p>",
		PlainTextBody: "User: {{username}}, Email: {{email}}, Extra: {{extra}}",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "user-789",
		RecipientEmail: "custom@example.com",
		Payload: map[string]any{
			"username": "Custom Name",              // overrides resolved username
			"email":    "custom.email@example.com", // overrides resolved email
			"extra":    "Extra Value",
		},
	}

	res, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected Create success, got: %v", err)
	}

	if res.Rendered.Subject != "Hello Custom Name" {
		t.Errorf("expected explicit username precedence 'Hello Custom Name', got '%s'", res.Rendered.Subject)
	}

	expectedHTML := "<p>User: Custom Name, Email: custom.email@example.com, Extra: Extra Value</p>"
	if res.Rendered.HTMLBody != expectedHTML {
		t.Errorf("expected HTML body '%s', got '%s'", expectedHTML, res.Rendered.HTMLBody)
	}
}

func TestService_CreateDirectNotification_UserVariables_HTMLEscaping(t *testing.T) {
	mockResolver := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return &userclient.User{
				ID:       id,
				Username: "<script>alert('xss')</script>",
				Email:    "test&user@example.com",
				Role:     "<b>Admin</b>",
				Language: "en",
			}, nil
		},
	}

	svc, tplRepo, _ := setupTestService(mockResolver)

	activeTpl := &domain.EmailTemplate{
		ID:            "tpl-html-escape",
		TemplateKey:   "security_test",
		Subject:       "User {{username}}",
		HTMLBody:      "<p>Name: {{username}}, Email: {{email}}, Role: {{role}}</p>",
		PlainTextBody: "Name: {{username}}, Email: {{email}}, Role: {{role}}",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	input := CreateInput{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "user-xss",
		RecipientEmail: "test@example.com",
	}

	res, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected Create success, got: %v", err)
	}

	// HTML body MUST be escaped
	expectedHTML := "<p>Name: &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;, Email: test&amp;user@example.com, Role: &lt;b&gt;Admin&lt;/b&gt;</p>"
	if res.Rendered.HTMLBody != expectedHTML {
		t.Errorf("expected escaped HTML '%s', got '%s'", expectedHTML, res.Rendered.HTMLBody)
	}

	// Plain text and subject MUST NOT be escaped
	if res.Rendered.Subject != "User <script>alert('xss')</script>" {
		t.Errorf("unexpected subject: %s", res.Rendered.Subject)
	}
	if res.Rendered.PlainTextBody != "Name: <script>alert('xss')</script>, Email: test&user@example.com, Role: <b>Admin</b>" {
		t.Errorf("unexpected plain text body: %s", res.Rendered.PlainTextBody)
	}
}

func TestMergeUserPayload_ApprovedFieldsOnly(t *testing.T) {
	u := &userclient.User{
		ID:               "secret-internal-id",
		Username:         "alice",
		Email:            "alice@example.com",
		Role:             "Admin",
		Language:         "en",
		IsActive:         true,
		IsBlocked:        false,
		RegistrationDate: time.Now().UTC(),
	}

	merged := MergeUserPayload(u, nil)

	// Ensure approved fields exist
	if merged["username"] != "alice" {
		t.Errorf("expected username alice, got %v", merged["username"])
	}
	if merged["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", merged["email"])
	}
	if merged["role"] != "Admin" {
		t.Errorf("expected role Admin, got %v", merged["role"])
	}
	if merged["language"] != "en" {
		t.Errorf("expected language en, got %v", merged["language"])
	}

	// Ensure sensitive/unapproved fields are NOT mapped into template variables
	unapprovedKeys := []string{"id", "ID", "password", "hash", "token", "isActive", "isBlocked", "registrationDate"}
	for _, key := range unapprovedKeys {
		if _, exists := merged[key]; exists {
			t.Errorf("unapproved key '%s' must not be exposed in template variables", key)
		}
	}
}

type mockActivityLogRepoForDirect struct {
	recordedLogs []*domain.ActivityLog
	createErr    error
}

func (m *mockActivityLogRepoForDirect) Create(ctx context.Context, log *domain.ActivityLog) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.recordedLogs = append(m.recordedLogs, log)
	return nil
}

func (m *mockActivityLogRepoForDirect) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepoForDirect) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func authContext(userID, role string) context.Context {
	p := &domain.Principal{UserID: userID, Role: role}
	return middleware.ContextWithPrincipal(context.Background(), p)
}

func TestDirectService_Audit_Create_Success(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:            "tpl-active-1",
				TemplateKey:   "order_update",
				Name:          "Order Update",
				TemplateType:  domain.TemplateTypeDirect,
				Subject:       "Order {{orderId}} is ready",
				HTMLBody:      "<p>Your order {{orderId}} is ready.</p>",
				PlainTextBody: "Your order {{orderId}} is ready.",
				Locale:        "en",
				Status:        domain.TemplateStatusActive,
				Version:       1,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	input := CreateInput{
		TemplateID:       "tpl-active-1",
		RecipientEmail:   "customer@example.com",
		RecipientName:    "John Doe",
		NotificationType: domain.NotificationTypeDirect,
		Payload: map[string]any{
			"orderId": "ORD-12345",
		},
	}

	res, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("expected successful creation, got %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventDirectCreated {
		t.Errorf("expected event type %q, got %q", domain.EventDirectCreated, log.EventType)
	}
	if log.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type %q, got %q", domain.ActorTypeUser, log.ActorType)
	}
	if log.ActorUserID == nil || *log.ActorUserID != "usr-admin-1" {
		t.Errorf("expected actor user ID 'usr-admin-1', got %v", log.ActorUserID)
	}
	if log.TargetType != domain.TargetTypeDirectNotification {
		t.Errorf("expected target type %q, got %q", domain.TargetTypeDirectNotification, log.TargetType)
	}
	if log.TargetID != res.Notification.ID {
		t.Errorf("expected target ID %q, got %q", res.Notification.ID, log.TargetID)
	}
	if log.TargetName == nil || *log.TargetName != "customer@example.com" {
		t.Errorf("expected target name 'customer@example.com', got %v", log.TargetName)
	}
	if log.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status %q, got %q", domain.ActivityStatusSuccess, log.Status)
	}
	if !strings.Contains(log.Summary, "customer@example.com") {
		t.Errorf("expected summary to contain recipient email, got %q", log.Summary)
	}

	var details directMessageDetails
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal audit details: %v", err)
	}
	if details.ID != res.Notification.ID {
		t.Errorf("expected details ID %q, got %q", res.Notification.ID, details.ID)
	}
	if details.TemplateID != "tpl-active-1" {
		t.Errorf("expected templateId 'tpl-active-1', got %q", details.TemplateID)
	}
	if details.TemplateKey != "order_update" {
		t.Errorf("expected templateKey 'order_update', got %q", details.TemplateKey)
	}
	if details.RecipientEmail != "customer@example.com" {
		t.Errorf("expected recipientEmail 'customer@example.com', got %q", details.RecipientEmail)
	}
	if details.RecipientName != "John Doe" {
		t.Errorf("expected recipientName 'John Doe', got %q", details.RecipientName)
	}
	if details.Subject != "Order ORD-12345 is ready" {
		t.Errorf("expected rendered subject, got %q", details.Subject)
	}
	if details.BodyHTML != "<p>Your order ORD-12345 is ready.</p>" {
		t.Errorf("expected rendered HTML body, got %q", details.BodyHTML)
	}
}

func TestDirectService_Audit_Create_FailedMutation_NoAudit(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Create(ctx, CreateInput{
		TemplateID:     "tpl-non-existent",
		RecipientEmail: "customer@example.com",
	})
	if err == nil {
		t.Fatal("expected error on non-existent template, got nil")
	}

	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs on failed creation, got %d", len(auditRepo.recordedLogs))
	}
}

func TestDirectService_Audit_Create_MissingPrincipal_ReturnsError(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:           "tpl-active-1",
				TemplateKey:  "welcome",
				Name:         "Welcome",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Welcome",
				HTMLBody:     "<p>Welcome</p>",
				Status:       domain.TemplateStatusActive,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, auditSvc)

	// Context without authenticated principal
	_, err := svc.Create(context.Background(), CreateInput{
		TemplateID:     "tpl-active-1",
		RecipientEmail: "customer@example.com",
	})
	if !errors.Is(err, audit.ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for unauthenticated context, got %v", err)
	}
}

func TestDirectService_Audit_Create_AuditFailure_PropagatesError(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-active-1": {
				ID:           "tpl-active-1",
				TemplateKey:  "welcome",
				Name:         "Welcome",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Welcome",
				HTMLBody:     "<p>Welcome</p>",
				Status:       domain.TemplateStatusActive,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &mockActivityLogRepoForDirect{
		createErr: errors.New("audit table write failed"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, auditSvc)

	ctx := authContext("usr-admin-1", "Admin")
	_, err := svc.Create(ctx, CreateInput{
		TemplateID:     "tpl-active-1",
		RecipientEmail: "customer@example.com",
	})
	if err == nil {
		t.Fatal("expected audit error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "audit table write failed") {
		t.Errorf("expected audit table write failed message, got: %v", err)
	}
}

func TestDirectService_SendNotification_Success(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-pw-en": {
				ID:           "tpl-pw-en",
				TemplateKey:  "auth.password_recovery",
				Name:         "Password Recovery (EN)",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Password Recovery",
				HTMLBody:     "<p>Reset link: {{ recoveryLink }}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
			"tpl-pw-ru": {
				ID:           "tpl-pw-ru",
				TemplateKey:  "auth.password_recovery",
				Name:         "Password Recovery (RU)",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Восстановление пароля",
				HTMLBody:     "<p>Ссылка: {{ recoveryLink }}</p>",
				Locale:       "ru",
				Status:       domain.TemplateStatusActive,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, auditSvc)

	ctx := authContext("svc-gmhelper-api", "Service")
	res, err := svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey:    "auth.password_recovery",
		Locale:         "ru",
		ExternalUserID: "user-123",
		RecipientEmail: "user@example.com",
		RecipientName:  "Test User",
		Variables: map[string]any{
			"recoveryLink": "https://gmhelper.com/recover?token=secret123",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Notification == nil {
		t.Fatal("expected notification to be returned")
	}
	if res.Notification.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected status 'pending', got '%s'", res.Notification.DeliveryStatus)
	}
	if res.Notification.RecipientEmail != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got '%s'", res.Notification.RecipientEmail)
	}
	if res.Notification.ExternalUserID != "user-123" {
		t.Errorf("expected externalUserId 'user-123', got '%s'", res.Notification.ExternalUserID)
	}
	if res.Rendered.Subject != "Восстановление пароля" {
		t.Errorf("expected rendered subject 'Восстановление пароля', got '%s'", res.Rendered.Subject)
	}

	// Verify persistence in directRepo
	saved, ok := directRepo.notifications[res.Notification.ID]
	if !ok {
		t.Fatal("expected notification to be saved in repository")
	}
	if saved.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected saved status 'pending', got '%s'", saved.DeliveryStatus)
	}

	// Verify initial attempt
	if len(directRepo.attempts) != 1 {
		t.Errorf("expected 1 delivery attempt, got %d", len(directRepo.attempts))
	}

	// Verify security: Audit details should NOT leak secret token in rendered body
	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", len(auditRepo.recordedLogs))
	}
	logEntry := auditRepo.recordedLogs[0]
	if strings.Contains(string(logEntry.Details), "secret123") {
		t.Errorf("audit log leaked sensitive token in body: %s", string(logEntry.Details))
	}
}

func TestDirectService_SendNotification_ValidationErrors(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-reg-en": {
				ID:           "tpl-reg-en",
				TemplateKey:  "auth.register_code",
				Name:         "Registration Code",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Your Code",
				HTMLBody:     "<p>Code: {{ code }}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
			"tpl-inactive": {
				ID:           "tpl-inactive",
				TemplateKey:  "auth.inactive_tpl",
				Name:         "Inactive",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Inactive",
				HTMLBody:     "<p>Inactive</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusDraft,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, nil)
	ctx := context.Background()

	// Missing templateKey
	_, err := svc.SendNotification(ctx, SendNotificationInput{
		RecipientEmail: "user@example.com",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing templateKey, got %v", err)
	}

	// Missing recipientEmail
	_, err = svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey: "auth.register_code",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing recipientEmail, got %v", err)
	}

	// Invalid email
	_, err = svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey:    "auth.register_code",
		RecipientEmail: "not-an-email",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid email, got %v", err)
	}

	// Unknown template
	_, err = svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey:    "non.existent",
		RecipientEmail: "user@example.com",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for non-existent template, got %v", err)
	}

	// Inactive template
	_, err = svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey:    "auth.inactive_tpl",
		RecipientEmail: "user@example.com",
	})
	if !errors.Is(err, ErrTemplateInactive) {
		t.Errorf("expected ErrTemplateInactive for draft template, got %v", err)
	}

	// Missing required variable
	_, err = svc.SendNotification(ctx, SendNotificationInput{
		TemplateKey:    "auth.register_code",
		RecipientEmail: "user@example.com",
		Variables:      map[string]any{}, // missing "code"
	})
	if !errors.Is(err, ErrMissingVariable) {
		t.Errorf("expected ErrMissingVariable when required placeholder is missing, got %v", err)
	}
}

func TestDirectService_SendNotification_DefaultLocaleAndWithoutExternalUserID(t *testing.T) {
	tplRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-reg-en": {
				ID:           "tpl-reg-en",
				TemplateKey:  "auth.register_code",
				Name:         "Registration Code",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Your Code",
				HTMLBody:     "<p>Code: {{ code }}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
		},
	}
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	svc := NewService(tplRepo, directRepo, &mockUserResolver{}, nil)

	res, err := svc.SendNotification(context.Background(), SendNotificationInput{
		TemplateKey:    "auth.register_code",
		Locale:         "", // empty, should default to "en"
		RecipientEmail: "newuser@example.com",
		Variables: map[string]any{
			"code": "123456",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Notification.TemplateID != "tpl-reg-en" {
		t.Errorf("expected template ID 'tpl-reg-en', got '%s'", res.Notification.TemplateID)
	}
	if res.Notification.ExternalUserID != "" {
		t.Errorf("expected empty ExternalUserID for registration, got '%s'", res.Notification.ExternalUserID)
	}
}
