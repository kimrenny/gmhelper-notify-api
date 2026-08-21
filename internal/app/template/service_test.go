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
		TemplateKey: "welcome_email",
		Name:        "Welcome Email",
		Subject:     "Welcome to GMHelper",
		HTMLBody:    "<h1>Welcome</h1>",
		Locale:      "en",
		Status:      "active",
		Version:     1,
	}

	created, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty generated ID")
	}

	// Get by ID
	fetched, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected get by ID success, got %v", err)
	}
	if fetched.Name != "Welcome Email" {
		t.Errorf("expected name 'Welcome Email', got %s", fetched.Name)
	}

	// Duplicate create conflict
	_, err = svc.Create(context.Background(), input)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestService_Create_InvalidInput(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	// Missing template key
	_, err := svc.Create(context.Background(), CreateInput{
		Name:     "No Key",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_Update_SuccessAndNotFound(t *testing.T) {
	repo := newMockTemplateRepo()
	svc := NewService(repo)

	t1 := &domain.EmailTemplate{
		ID:          "tpl-1",
		TemplateKey: "key_1",
		Name:        "Name 1",
		Subject:     "Subject 1",
		HTMLBody:    "<p>1</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.templates[t1.ID] = t1

	// Update existing
	updated, err := svc.Update(context.Background(), "tpl-1", UpdateInput{
		TemplateKey: "key_1_updated",
		Name:        "Name 1 Updated",
		Subject:     "Subject 1 Updated",
		HTMLBody:    "<p>1 updated</p>",
		Locale:      "en",
		Status:      "draft",
		Version:     2,
	})
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if updated.Name != "Name 1 Updated" {
		t.Errorf("expected updated name, got %s", updated.Name)
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
		ID:          "tpl-1",
		TemplateKey: "key_1",
		Name:        "Name 1",
		Subject:     "Subject 1",
		HTMLBody:    "<p>1</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
	}
	repo.templates[t1.ID] = t1

	if err := svc.Delete(context.Background(), "tpl-1"); err != nil {
		t.Fatalf("expected delete success, got %v", err)
	}

	if err := svc.Delete(context.Background(), "tpl-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}
