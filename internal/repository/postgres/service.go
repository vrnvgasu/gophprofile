// Package postgres реализует хранилище метаданных аватарок на основе PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Storage struct {
	db *sql.DB
}

func NewStorage() *Storage {
	return &Storage{}
}

func (s *Storage) Start(ctx context.Context, dsn string) error {
	classifier := newPostgresErrorClassifier()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres.Start Open: %w", err)
	}

	s.db = db

	err = withRetry(func() error { return s.db.PingContext(ctx) }, classifier)
	if err != nil {
		return fmt.Errorf("postgres.Start Ping: %w", err)
	}

	err = withRetry(func() error { return s.migrate(ctx) }, classifier)
	if err != nil {
		return fmt.Errorf("postgres.Start Migrate: %w", err)
	}

	return nil
}

func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Storage) Stop() error {
	return s.db.Close()
}
