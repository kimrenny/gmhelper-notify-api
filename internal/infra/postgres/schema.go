package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// ApplyMigrations applies all pending embedded SQL migrations in deterministic order.
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	return ApplyMigrationsFromFS(ctx, db, embeddedMigrations, "migrations")
}

// ApplyMigrationsFromFS applies all pending migrations from the provided filesystem in deterministic order.
func ApplyMigrationsFromFS(ctx context.Context, db *sql.DB, fsys fs.FS, dir string) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory %q: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] == ".sql" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		if applied[filename] {
			continue
		}

		filePath := filename
		if dir != "" && dir != "." {
			filePath = dir + "/" + filename
		}

		content, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		if err := applyMigration(ctx, db, filename, string(content)); err != nil {
			return fmt.Errorf("migration %s failed: %w", filename, err)
		}
	}

	return nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`
	_, err := db.ExecContext(ctx, query)
	return err
}

func getAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func applyMigration(ctx context.Context, db *sql.DB, version, content string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, content); err != nil {
		return fmt.Errorf("failed to execute migration content: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, now())`, version); err != nil {
		return fmt.Errorf("failed to record migration status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
