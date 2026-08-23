package postgres

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

func NewPostgresDB(ctx context.Context, databaseURL string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *PostgresDB) Close(ctx context.Context) error {
	return p.db.Close()
}

func (p *PostgresDB) DB() *sql.DB {
	return p.db
}
