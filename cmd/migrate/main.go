// Команда migrate применяет миграции БД и завершается.
// Используется хуком Helm перед выкаткой новой версии сервиса.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/config"
	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/repository/postgres"
)

const timeout = 2 * time.Minute

func main() {
	if err := run(); err != nil {
		logger.Log.Error("migrate stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse()
	if err != nil {
		return fmt.Errorf("config.Parse: %w", err)
	}

	if err = logger.Initialize(cfg.LogLevel, "migrate"); err != nil {
		return fmt.Errorf("logger.Initialize: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Storage.Start сам применяет миграции goose после успешного Ping.
	storage := postgres.NewStorage()
	if err = storage.Start(ctx, cfg.DatabaseURI); err != nil {
		return fmt.Errorf("storage.Start: %w", err)
	}
	defer func() { _ = storage.Stop() }()

	logger.Log.Info("migrations applied")

	return nil
}
