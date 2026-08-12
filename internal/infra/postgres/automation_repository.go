package postgres

import (
	"context"
	"database/sql"

	"github.com/gmhelper/notify-api/internal/domain"
)

type AutomationRuleRepository struct {
	db *sql.DB
}

func NewAutomationRuleRepository(db *sql.DB) *AutomationRuleRepository {
	return &AutomationRuleRepository{db: db}
}

func (r *AutomationRuleRepository) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	rule := &domain.AutomationRule{}
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, event_type, template_id, enabled, config, created_at, updated_at
FROM automation_rules
WHERE id = $1`, id)
	if err := row.Scan(&rule.ID, &rule.Name, &rule.EventType, &rule.TemplateID, &rule.Enabled, &rule.Config, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return rule, nil
}

func (r *AutomationRuleRepository) Create(ctx context.Context, rule *domain.AutomationRule) error {
	if err := rule.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO automation_rules (id, name, event_type, template_id, enabled, config, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, rule.ID, rule.Name, rule.EventType, rule.TemplateID, rule.Enabled, rule.Config, rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (r *AutomationRuleRepository) Update(ctx context.Context, rule *domain.AutomationRule) error {
	if err := rule.Validate(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE automation_rules
SET name = $1, event_type = $2, template_id = $3, enabled = $4, config = $5, updated_at = now()
WHERE id = $6`, rule.Name, rule.EventType, rule.TemplateID, rule.Enabled, rule.Config, rule.ID)
	return err
}

func (r *AutomationRuleRepository) ListEnabled(ctx context.Context) ([]*domain.AutomationRule, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, event_type, template_id, enabled, config, created_at, updated_at
FROM automation_rules
WHERE enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []*domain.AutomationRule{}
	for rows.Next() {
		rule := &domain.AutomationRule{}
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.EventType, &rule.TemplateID, &rule.Enabled, &rule.Config, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
