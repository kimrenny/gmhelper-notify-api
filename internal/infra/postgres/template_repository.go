package postgres

import (
	"context"
	"database/sql"

	"github.com/gmhelper/notify-api/internal/domain"
)

type EmailTemplateRepository struct {
	db *sql.DB
}

func NewEmailTemplateRepository(db *sql.DB) *EmailTemplateRepository {
	return &EmailTemplateRepository{db: db}
}

func (r *EmailTemplateRepository) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	template := &domain.EmailTemplate{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at
FROM email_templates
WHERE id = $1`, id)
	if err := row.Scan(&template.ID, &template.TemplateKey, &template.Name, &template.Subject, &template.HTMLBody, &template.PlainTextBody, &template.Locale, &template.Status, &template.Version, &template.CreatedAt, &template.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return template, nil
}

func (r *EmailTemplateRepository) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	template := &domain.EmailTemplate{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at
FROM email_templates
WHERE template_key = $1
ORDER BY locale, version DESC
LIMIT 1`, templateKey)
	if err := row.Scan(&template.ID, &template.TemplateKey, &template.Name, &template.Subject, &template.HTMLBody, &template.PlainTextBody, &template.Locale, &template.Status, &template.Version, &template.CreatedAt, &template.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return template, nil
}

func (r *EmailTemplateRepository) Create(ctx context.Context, template *domain.EmailTemplate) error {
	if err := template.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO email_templates (id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, template.ID, template.TemplateKey, template.Name, template.Subject, template.HTMLBody, template.PlainTextBody, template.Locale, template.Status, template.Version, template.CreatedAt, template.UpdatedAt)
	return err
}

func (r *EmailTemplateRepository) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, template_key, name, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at
FROM email_templates
ORDER BY template_key, locale, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []*domain.EmailTemplate{}
	for rows.Next() {
		template := &domain.EmailTemplate{}
		if err := rows.Scan(&template.ID, &template.TemplateKey, &template.Name, &template.Subject, &template.HTMLBody, &template.PlainTextBody, &template.Locale, &template.Status, &template.Version, &template.CreatedAt, &template.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, rows.Err()
}
