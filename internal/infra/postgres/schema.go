package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"path/filepath"
)

func ApplyMigrations(ctx context.Context, db *sql.DB, migrationsPath string) error {
	files, err := filepath.Glob(filepath.Join(migrationsPath, "*.sql"))
	if err != nil {
		return err
	}
	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("migration %s failed: %w", filepath.Base(file), err)
		}
	}
	return nil
}
