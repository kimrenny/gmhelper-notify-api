package template

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

type mockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
	err       error
}

func newMockTemplateRepo() *mockTemplateRepo {
	return &mockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
}

func (m *mockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}

func (m *mockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, t := range m.templates {
		if t.TemplateKey == templateKey {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	if m.err != nil {
		return m.err
	}
	for _, t := range m.templates {
		if t.TemplateKey == template.TemplateKey && t.Locale == template.Locale && t.Version == template.Version {
			return domain.ErrConflict
		}
	}
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.templates[template.ID]; !ok {
		return domain.ErrNotFound
	}
	for _, t := range m.templates {
		if t.ID != template.ID && t.TemplateKey == template.TemplateKey && t.Locale == template.Locale && t.Version == template.Version {
			return domain.ErrConflict
		}
	}
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Delete(ctx context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.templates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.templates, id)
	return nil
}

func (m *mockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	if m.err != nil {
		return nil, m.err
	}
	list := make([]*domain.EmailTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		list = append(list, t)
	}
	return list, nil
}

func TestService_CreateAndGet(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	// Valid create
	input := CreateInput{
		TemplateKey:  "welcome_email",
		Name:         "Welcome Email",
		TemplateType: "direct",
		Subject:      "Welcome to GMHelper",
		HTMLBody:     "<h1>Welcome</h1>",
		Locale:       "en",
		Status:       "active",
		Version:      1,
	}

	created, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty generated ID")
	}
	if created.TemplateType != domain.TemplateTypeDirect {
		t.Errorf("expected template type 'direct', got %s", created.TemplateType)
	}

	// Get by ID
	fetched, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected get by ID success, got %v", err)
	}
	if fetched.Name != "Welcome Email" {
		t.Errorf("expected name 'Welcome Email', got %s", fetched.Name)
	}
	if fetched.TemplateType != domain.TemplateTypeDirect {
		t.Errorf("expected fetched template type 'direct', got %s", fetched.TemplateType)
	}

	// Duplicate create conflict
	_, err = svc.Create(context.Background(), input)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestService_Create_AllSupportedTypes(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	types := []domain.TemplateType{
		domain.TemplateTypeDirect,
		domain.TemplateTypeCampaign,
		domain.TemplateTypeUserAgreement,
		domain.TemplateTypeAutomation,
	}

	for _, typ := range types {
		input := CreateInput{
			TemplateKey:  "tpl_" + string(typ),
			Name:         "Template " + string(typ),
			TemplateType: string(typ),
			Subject:      "Subject",
			HTMLBody:     "<p>Body</p>",
			Locale:       "en",
			Status:       "active",
			Version:      1,
		}

		created, err := svc.Create(context.Background(), input)
		if err != nil {
			t.Fatalf("expected create success for type %s, got %v", typ, err)
		}
		if created.TemplateType != typ {
			t.Errorf("expected type %s, got %s", typ, created.TemplateType)
		}

		fetched, err := svc.GetByID(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("expected get success for type %s, got %v", typ, err)
		}
		if fetched.TemplateType != typ {
			t.Errorf("expected fetched type %s, got %s", typ, fetched.TemplateType)
		}
	}
}

func TestService_Create_InvalidType(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	// Missing template type
	_, err := svc.Create(context.Background(), CreateInput{
		TemplateKey:  "tpl_missing_type",
		Name:         "Missing Type",
		TemplateType: "",
		Subject:      "Subject",
		HTMLBody:     "<p>Body</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty type, got %v", err)
	}

	// Unsupported template type
	_, err = svc.Create(context.Background(), CreateInput{
		TemplateKey:  "tpl_invalid_type",
		Name:         "Invalid Type",
		TemplateType: "marketing",
		Subject:      "Subject",
		HTMLBody:     "<p>Body</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid type, got %v", err)
	}
}

func TestService_Create_InvalidInput(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	// Missing template key
	_, err := svc.Create(context.Background(), CreateInput{
		Name:         "No Key",
		TemplateType: "campaign",
		Subject:      "Subject",
		HTMLBody:     "<p>Body</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_Update_SuccessAndNotFound(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	t1 := &domain.EmailTemplate{
		ID:           "tpl-1",
		TemplateKey:  "key_1",
		Name:         "Name 1",
		TemplateType: domain.TemplateTypeCampaign,
		Subject:      "Subject 1",
		HTMLBody:     "<p>1</p>",
		Locale:       "en",
		Status:       domain.TemplateStatusActive,
		Version:      1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	repo.templates[t1.ID] = t1

	// Update existing with matching type
	updated, err := svc.Update(context.Background(), "tpl-1", UpdateInput{
		TemplateKey:  "key_1_updated",
		Name:         "Name 1 Updated",
		TemplateType: "campaign",
		Subject:      "Subject 1 Updated",
		HTMLBody:     "<p>1 updated</p>",
		Locale:       "en",
		Status:       "draft",
		Version:      2,
	})
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if updated.Name != "Name 1 Updated" {
		t.Errorf("expected updated name, got %s", updated.Name)
	}
	if updated.TemplateType != domain.TemplateTypeCampaign {
		t.Errorf("expected preserved template type 'campaign', got %s", updated.TemplateType)
	}

	// Update existing without specifying type (empty preserves existing)
	updated2, err := svc.Update(context.Background(), "tpl-1", UpdateInput{
		TemplateKey: "key_1_updated",
		Name:        "Name 1 Updated Again",
		Subject:     "Subject 1 Updated",
		HTMLBody:    "<p>1 updated</p>",
		Locale:      "en",
		Status:      "draft",
		Version:     3,
	})
	if err != nil {
		t.Fatalf("expected update success without type, got %v", err)
	}
	if updated2.TemplateType != domain.TemplateTypeCampaign {
		t.Errorf("expected preserved template type 'campaign', got %s", updated2.TemplateType)
	}

	// Attempting to change type to a different type fails
	_, err = svc.Update(context.Background(), "tpl-1", UpdateInput{
		TemplateKey:  "key_1_updated",
		Name:         "Name 1",
		TemplateType: "direct",
		Subject:      "Subject 1",
		HTMLBody:     "<p>1</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput when trying to change immutable type, got %v", err)
	}

	// Attempting to change type to an invalid type fails
	_, err = svc.Update(context.Background(), "tpl-1", UpdateInput{
		TemplateKey:  "key_1_updated",
		Name:         "Name 1",
		TemplateType: "invalid_type",
		Subject:      "Subject 1",
		HTMLBody:     "<p>1</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid type on update, got %v", err)
	}

	// Update non-existing
	_, err = svc.Update(context.Background(), "tpl-999", UpdateInput{
		TemplateKey: "key_999",
		Name:        "Name 999",
		Subject:     "Subject 999",
		HTMLBody:    "<p>999</p>",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_SuccessAndNotFound(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	t1 := &domain.EmailTemplate{
		ID:           "tpl-1",
		TemplateKey:  "key_1",
		Name:         "Name 1",
		TemplateType: domain.TemplateTypeDirect,
		Subject:      "Subject 1",
		HTMLBody:     "<p>1</p>",
		Locale:       "en",
		Status:       domain.TemplateStatusActive,
		Version:      1,
	}
	repo.templates[t1.ID] = t1

	if err := svc.Delete(context.Background(), "tpl-1"); err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}

	if err := svc.Delete(context.Background(), "tpl-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestService_Preview_SuccessAndHTMLEscaping(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	tpl := &domain.EmailTemplate{
		ID:            "tpl-preview-1",
		TemplateKey:   "welcome_user",
		Name:          "Welcome Template",
		TemplateType:  domain.TemplateTypeDirect,
		Subject:       "Welcome, {{username}}!",
		HTMLBody:      "<h1>Hello, {{username}}!</h1><p>Email: {{email}}</p>",
		PlainTextBody: "Hello, {{username}}! Email: {{email}}",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	repo.templates[tpl.ID] = tpl

	// 1. Success with variable substitution and HTML escaping
	vars := map[string]any{
		"username": "<b>John & Jane</b>",
		"email":    "john@example.com",
	}

	res, err := svc.Preview(context.Background(), "tpl-preview-1", PreviewInput{Variables: vars})
	if err != nil {
		t.Fatalf("expected preview success, got %v", err)
	}

	// Subject: raw replacement
	if res.Subject != "Welcome, <b>John & Jane</b>!" {
		t.Errorf("expected raw subject, got: %s", res.Subject)
	}

	// HTMLBody: HTML-escaped variables
	expectedHTML := "<h1>Hello, &lt;b&gt;John &amp; Jane&lt;/b&gt;!</h1><p>Email: john@example.com</p>"
	if res.HTMLBody != expectedHTML {
		t.Errorf("expected escaped HTML body '%s', got '%s'", expectedHTML, res.HTMLBody)
	}

	// PlainTextBody: raw replacement
	expectedPlain := "Hello, <b>John & Jane</b>! Email: john@example.com"
	if res.PlainTextBody != expectedPlain {
		t.Errorf("expected plain text '%s', got '%s'", expectedPlain, res.PlainTextBody)
	}
}

func TestService_Preview_UnsavedOverrides(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	subject := "Unsaved Subject {{name}}"
	htmlBody := "<div>Unsaved HTML {{name}}</div>"
	plainText := "Unsaved Plain {{name}}"

	res, err := svc.Preview(context.Background(), "preview", PreviewInput{
		Subject:       &subject,
		HTMLBody:      &htmlBody,
		PlainTextBody: &plainText,
		Variables: map[string]any{
			"name": "SuperUser",
		},
	})
	if err != nil {
		t.Fatalf("expected preview success with overrides, got: %v", err)
	}

	if res.Subject != "Unsaved Subject SuperUser" {
		t.Errorf("unexpected subject: %s", res.Subject)
	}
	if res.HTMLBody != "<div>Unsaved HTML SuperUser</div>" {
		t.Errorf("unexpected htmlBody: %s", res.HTMLBody)
	}
}

func TestService_Preview_ExistingTemplateIDWithOverrides(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	tpl := &domain.EmailTemplate{
		ID:            "tpl-existing-100",
		TemplateKey:   "existing_key",
		Name:          "Existing Name",
		TemplateType:  domain.TemplateTypeCampaign,
		Subject:       "Persisted DB Subject",
		HTMLBody:      "<p>Persisted DB HTML</p>",
		PlainTextBody: "Persisted DB PlainText",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	repo.templates[tpl.ID] = tpl

	// 1. Override both subject and HTML body on existing template
	overrideSubject := "Live Unsaved Subject {{name}}"
	overrideHTML := "<div>Live Unsaved HTML {{name}}</div>"

	res, err := svc.Preview(context.Background(), "tpl-existing-100", PreviewInput{
		Subject:  &overrideSubject,
		HTMLBody: &overrideHTML,
		Variables: map[string]any{
			"name": "Alex",
		},
	})
	if err != nil {
		t.Fatalf("expected preview success with overrides on existing template, got %v", err)
	}

	if res.Subject != "Live Unsaved Subject Alex" {
		t.Errorf("expected overridden subject 'Live Unsaved Subject Alex', got '%s'", res.Subject)
	}
	if res.HTMLBody != "<div>Live Unsaved HTML Alex</div>" {
		t.Errorf("expected overridden HTML body '<div>Live Unsaved HTML Alex</div>', got '%s'", res.HTMLBody)
	}
	if res.PlainTextBody != "Persisted DB PlainText" {
		t.Errorf("expected fallback DB plain text, got '%s'", res.PlainTextBody)
	}

	// 2. Override only subject
	singleOverrideSubject := "Single Subject Override"
	res2, err := svc.Preview(context.Background(), "tpl-existing-100", PreviewInput{
		Subject: &singleOverrideSubject,
	})
	if err != nil {
		t.Fatalf("expected preview success with single override, got %v", err)
	}
	if res2.Subject != "Single Subject Override" {
		t.Errorf("expected overridden subject, got '%s'", res2.Subject)
	}
	if res2.HTMLBody != "<p>Persisted DB HTML</p>" {
		t.Errorf("expected DB HTML body, got '%s'", res2.HTMLBody)
	}
}

func TestService_Preview_MissingVariablesAndNotFound(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	tpl := &domain.EmailTemplate{
		ID:           "tpl-preview-2",
		TemplateKey:  "req_vars",
		Name:         "Required Vars",
		TemplateType: domain.TemplateTypeDirect,
		Subject:      "Subject {{code}}",
		HTMLBody:     "<p>{{code}}</p>",
		Locale:       "en",
		Status:       domain.TemplateStatusActive,
		Version:      1,
	}
	repo.templates[tpl.ID] = tpl

	// 1. Missing variable returns ErrMissingVariable
	_, err := svc.Preview(context.Background(), "tpl-preview-2", PreviewInput{Variables: map[string]any{}})
	if !errors.Is(err, ErrMissingVariable) {
		t.Fatalf("expected ErrMissingVariable, got %v", err)
	}

	// 2. Non-existent template returns ErrNotFound
	_, err = svc.Preview(context.Background(), "non-existent-id", PreviewInput{Variables: map[string]any{"code": "123"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. Empty ID returns ErrInvalidInput
	_, err = svc.Preview(context.Background(), "", PreviewInput{Variables: map[string]any{"code": "123"}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
