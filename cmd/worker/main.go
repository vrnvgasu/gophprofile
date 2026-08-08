// Команда worker обрабатывает события об аватарках из Kafka:
// создает миниатюры и удаляет файлы из объектного хранилища.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vrnvgasu/gophprofile/internal/broker/kafka"
	"github.com/vrnvgasu/gophprofile/internal/config"
	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/repository/postgres"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/internal/worker"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("config.Parse: %v", err)
	}

	if err = logger.Initialize(cfg.LogLevel); err != nil {
		log.Fatalf("logger.Initialize: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	storage := postgres.NewStorage()
	if err = storage.Start(ctx, cfg.DatabaseURI); err != nil {
		logger.Log.Fatalf("storage.Start: %v", err)
	}
	defer func() { _ = storage.Stop() }()

	objects, err := s3.NewStorage(ctx, s3.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		logger.Log.Fatalf("s3.NewStorage: %v", err)
	}

	consumer, err := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID)
	if err != nil {
		logger.Log.Fatalf("kafka.NewConsumer: %v", err)
	}
	defer consumer.Close()

	w := worker.New(storage, objects)

	logger.Log.Infow("starting worker",
		"topic", cfg.KafkaTopic, "group", cfg.KafkaGroupID, "brokers", cfg.KafkaBrokers)

	consumer.Run(ctx, w.Handle)

	logger.Log.Info("worker stopped")
}
