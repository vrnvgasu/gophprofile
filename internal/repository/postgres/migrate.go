package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/vrnvgasu/gophprofile/migrations"
)

const migrateTimeout = 30 * time.Second

func (s *Storage) migrate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, migrateTimeout)
	defer cancel()

	goose.SetBaseFS(migrations.MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres.migrate SetDialect: %w", err)
	}

	if err := goose.UpContext(ctx, s.db, "."); err != nil {
		return fmt.Errorf("postgres.migrate Up: %w", err)
	}

	return nil
}
