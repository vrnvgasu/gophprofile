package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/broker/kafka"
	"github.com/vrnvgasu/gophprofile/internal/config"
	"github.com/vrnvgasu/gophprofile/internal/handler"
	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/metrics"
	"github.com/vrnvgasu/gophprofile/internal/repository/postgres"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/internal/telemetry"
)

const (
	shutdownTimeout = 5 * time.Second
	serviceName     = "gophprofile-server"
	version         = "1.0.0"
)

func main() {
	if err := run(); err != nil {
		logger.Log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse()
	if err != nil {
		return fmt.Errorf("config.Parse: %w", err)
	}

	if err = logger.Initialize(cfg.LogLevel, "server"); err != nil {
		return fmt.Errorf("logger.Initialize: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, serviceName, version, cfg.OTLPEndpoint, cfg.TraceSampleRate)
	if err != nil {
		return fmt.Errorf("telemetry.Init: %w", err)
	}

	logger.AttachOTLP(serviceName)

	storage := postgres.NewStorage()
	if err = storage.Start(ctx, cfg.DatabaseURI); err != nil {
		return fmt.Errorf("storage.Start: %w", err)
	}
	defer func() { _ = storage.Stop() }()

	defer func() {
		if shutdownErr := shutdownTelemetry(context.Background()); shutdownErr != nil {
			logger.Log.Error("telemetry shutdown", "error", shutdownErr)
		}
	}()

	if err = metrics.RegisterDBStats(storage.DB(), "gophprofile"); err != nil {
		return fmt.Errorf("metrics.RegisterDBStats: %w", err)
	}

	if err = metrics.RegisterStats(storage); err != nil {
		return fmt.Errorf("metrics.RegisterStats: %w", err)
	}

	objects, err := s3.NewStorage(ctx, s3.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		return fmt.Errorf("s3.NewStorage: %w", err)
	}

	producer, err := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		return fmt.Errorf("kafka.NewProducer: %w", err)
	}
	defer producer.Close()

	service := avatar.NewService(storage, objects, producer, cfg.MaxUploadSize)
	h := handler.NewHandler(service, cfg.MaxUploadSize)
	router := handler.NewRouter(h, cfg.StaticDir)

	srv := &http.Server{
		Addr:              cfg.RunAddress,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Ошибка слушателя должна вернуться в run, а не завершать процесс из горутины.
	srvErr := make(chan error, 1)

	logger.Log.Info("starting server", "address", cfg.RunAddress)
	metrics.MarkStarted()

	go func() {
		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			srvErr <- fmt.Errorf("srv.ListenAndServe: %w", listenErr)
		}
	}()

	select {
	case err = <-srvErr:
		return err
	case <-ctx.Done():
	}

	logger.Log.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("srv.Shutdown: %w", err)
	}

	return nil
}
