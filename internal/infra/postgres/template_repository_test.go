package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestEmailTemplateRepository_CreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewEmailTemplateRepository(db)
	template := &domain.EmailTemplate{
		ID:            "f7ba18f7-4c2a-4b77-8565-1e1e5d64047f",
		TemplateKey:   "welcome_email",
		Name:          "Welcome Email",
		Subject:       "Welcome",
		HTMLBody:      "<p>Hello</p>",
		Locale:        "en-US",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO email_templates (id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)).
		WithArgs(template.ID, template.TemplateKey, template.Name, template.Subject, template.HTMLBody, template.PlainTextBody, template.Locale, template.Status, template.Version, template.CreatedAt, template.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), template); err != nil {
		t.Fatalf("failed to create template: %v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "template_key", "name", "subject", "html_body", "plain_text_body", "locale", "status", "version", "created_at", "updated_at"}).
		AddRow(template.ID, template.TemplateKey, template.Name, template.Subject, template.HTMLBody, template.PlainTextBody, template.Locale, template.Status, template.Version, template.CreatedAt, template.UpdatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at
FROM email_templates
WHERE id = $1`)).
		WithArgs(template.ID).
		WillReturnRows(rows)

	fetched, err := repo.GetByID(context.Background(), template.ID)
	if err != nil {
		t.Fatalf("failed to fetch template: %v", err)
	}
	if fetched.TemplateKey != template.TemplateKey {
		t.Fatalf("expected template key %s, got %s", template.TemplateKey, fetched.TemplateKey)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestEmailTemplateRepository_UpdateAndDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewEmailTemplateRepository(db)
	template := &domain.EmailTemplate{
		ID:            "f7ba18f7-4c2a-4b77-8565-1e1e5d64047f",
		TemplateKey:   "welcome_email",
		Name:          "Welcome Email Updated",
		Subject:       "Welcome Updated",
		HTMLBody:      "<p>Hello updated</p>",
		Locale:        "en-US",
		Status:        domain.TemplateStatusActive,
		Version:       2,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// 1. Update
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE email_templates
SET template_key = $1, name = $2, subject = $3, html_body = $4, plain_text_body = $5, locale = $6, status = $7, version = $8, updated_at = $9
WHERE id = $10`)).
		WithArgs(template.TemplateKey, template.Name, template.Subject, template.HTMLBody, template.PlainTextBody, template.Locale, template.Status, template.Version, template.UpdatedAt, template.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Update(context.Background(), template); err != nil {
		t.Fatalf("failed to update template: %v", err)
	}

	// 2. Delete
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM email_templates WHERE id = $1`)).
		WithArgs(template.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Delete(context.Background(), template.ID); err != nil {
		t.Fatalf("failed to delete template: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

