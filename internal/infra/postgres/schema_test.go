package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyMigrationsFromFS_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	mockFS := fstest.MapFS{
		"migrations/0001_init.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE dummy (id text);"),
		},
		"migrations/0002_add_index.sql": &fstest.MapFile{
			Data: []byte("CREATE INDEX idx_dummy ON dummy(id);"),
		},
	}

	// 1. Ensure table
	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS schema_migrations`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 2. Query applied: 0001_init.sql already applied
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT version FROM schema_migrations`)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("0001_init.sql"))

	// 3. 0002_add_index.sql must be applied in a transaction
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`CREATE INDEX idx_dummy ON dummy(id);`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, applied_at) VALUES ($1, now())`)).
		WithArgs("0002_add_index.sql").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = ApplyMigrationsFromFS(context.Background(), db, mockFS, "migrations")
	if err != nil {
		t.Fatalf("expected migrations to succeed, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestApplyMigrationsFromFS_RollbackOnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	mockFS := fstest.MapFS{
		"migrations/0001_broken.sql": &fstest.MapFile{
			Data: []byte("INVALID SQL SYNTAX;"),
		},
	}

	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS schema_migrations`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT version FROM schema_migrations`)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}))

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INVALID SQL SYNTAX;`)).
		WillReturnError(errors.New("syntax error"))
	mock.ExpectRollback()

	err = ApplyMigrationsFromFS(context.Background(), db, mockFS, "migrations")
	if err == nil {
		t.Fatal("expected migration failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
