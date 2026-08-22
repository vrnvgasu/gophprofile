// Команда worker обрабатывает события об аватарках из Kafka:
// создает миниатюры и удаляет файлы из объектного хранилища.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/broker/kafka"
	"github.com/vrnvgasu/gophprofile/internal/config"
	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/metrics"
	"github.com/vrnvgasu/gophprofile/internal/repository/postgres"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/internal/telemetry"
	"github.com/vrnvgasu/gophprofile/internal/worker"
)

const (
	lagInterval = 10 * time.Second
	serviceName = "gophprofile-worker"
	version     = "1.0.0"
)

func main() {
	if err := run(); err != nil {
		logger.Log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse()
	if err != nil {
		return fmt.Errorf("config.Parse: %w", err)
	}

	if err = logger.Initialize(cfg.LogLevel, "worker"); err != nil {
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

	consumer, err := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID)
	if err != nil {
		return fmt.Errorf("kafka.NewConsumer: %w", err)
	}
	defer consumer.Close()

	go consumer.WatchLag(ctx, cfg.KafkaGroupID, lagInterval)

	w := worker.New(storage, objects)

	logger.Log.Info("starting worker",
		"topic", cfg.KafkaTopic, "group", cfg.KafkaGroupID, "brokers", cfg.KafkaBrokers)
	metrics.MarkStarted()

	consumer.Run(ctx, w.Handle)

	logger.Log.Info("worker stopped")

	return nil
}
