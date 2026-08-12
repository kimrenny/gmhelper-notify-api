package postgres

import (
	"context"
	"database/sql"

	"github.com/gmhelper/notify-api/internal/domain"
)

type AppSettingRepository struct {
	db *sql.DB
}

func NewAppSettingRepository(db *sql.DB) *AppSettingRepository {
	return &AppSettingRepository{db: db}
}

func (r *AppSettingRepository) GetByKey(ctx context.Context, key string) (*domain.AppSetting, error) {
	setting := &domain.AppSetting{}
	row := r.db.QueryRowContext(ctx, `
SELECT key, value, category, description, created_at, updated_at
FROM app_settings
WHERE key = $1`, key)
	if err := row.Scan(&setting.Key, &setting.Value, &setting.Category, &setting.Description, &setting.CreatedAt, &setting.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return setting, nil
}

func (r *AppSettingRepository) Save(ctx context.Context, setting *domain.AppSetting) error {
	if setting.Key == "" || setting.Value == "" || setting.Category == "" {
		return domain.ErrInvalidEntity
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value, category, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, category = EXCLUDED.category, description = EXCLUDED.description, updated_at = now()`, setting.Key, setting.Value, setting.Category, setting.Description)
	return err
}
