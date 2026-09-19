package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
)

func TestAppSettingRepository_SaveAndGetByKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAppSettingRepository(db)
	now := time.Now().UTC()
	setting := &domain.AppSetting{
		Key:         "notification.default_from_name",
		Value:       "GMHelper Notifications",
		Category:    "notification",
		Description: "Default sender name",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 1. Save (Insert/Upsert)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO app_settings (key, value, category, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, category = EXCLUDED.category, description = EXCLUDED.description, updated_at = now()`)).
		WithArgs(setting.Key, setting.Value, setting.Category, setting.Description).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Save(context.Background(), setting); err != nil {
		t.Fatalf("failed to save setting: %v", err)
	}

	// 2. GetByKey
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT key, value, category, description, created_at, updated_at
FROM app_settings
WHERE key = $1`)).
		WithArgs(setting.Key).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "category", "description", "created_at", "updated_at"}).
			AddRow(setting.Key, setting.Value, setting.Category, setting.Description, setting.CreatedAt, setting.UpdatedAt))

	fetched, err := repo.GetByKey(context.Background(), setting.Key)
	if err != nil {
		t.Fatalf("failed to fetch setting by key: %v", err)
	}
	if fetched.Key != setting.Key || fetched.Value != setting.Value || fetched.Category != setting.Category {
		t.Fatalf("unexpected fetched setting: %+v", fetched)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAppSettingRepository_ValidationAndNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAppSettingRepository(db)

	// Invalid entities
	if err := repo.Save(context.Background(), &domain.AppSetting{Key: "", Category: "cat"}); err != domain.ErrInvalidEntity {
		t.Errorf("expected ErrInvalidEntity for empty key, got: %v", err)
	}
	if err := repo.Save(context.Background(), &domain.AppSetting{Key: "key", Category: ""}); err != domain.ErrInvalidEntity {
		t.Errorf("expected ErrInvalidEntity for empty category, got: %v", err)
	}

	// Not found
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT key, value, category, description, created_at, updated_at
FROM app_settings
WHERE key = $1`)).
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "category", "description", "created_at", "updated_at"}))

	_, err = repo.GetByKey(context.Background(), "nonexistent")
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for missing key, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestAppSettingRepository_ListByCategoryAndListAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewAppSettingRepository(db)
	now := time.Now().UTC()

	// 1. ListByCategory
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT key, value, category, description, created_at, updated_at
FROM app_settings
WHERE category = $1
ORDER BY key ASC`)).
		WithArgs("notification").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "category", "description", "created_at", "updated_at"}).
			AddRow("notification.default_from_name", "GMHelper", "notification", "From name", now, now).
			AddRow("notification.default_locale", "en", "notification", "Default locale", now, now))

	notifSettings, err := repo.ListByCategory(context.Background(), "notification")
	if err != nil {
		t.Fatalf("failed to list settings by category: %v", err)
	}
	if len(notifSettings) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(notifSettings))
	}

	// 2. ListAll
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT key, value, category, description, created_at, updated_at
FROM app_settings
ORDER BY category ASC, key ASC`)).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "category", "description", "created_at", "updated_at"}).
			AddRow("notification.default_from_name", "GMHelper", "notification", "From name", now, now).
			AddRow("system.maintenance", "false", "system", "Maintenance flag", now, now))

	allSettings, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("failed to list all settings: %v", err)
	}
	if len(allSettings) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(allSettings))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
