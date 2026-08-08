package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/broker/kafka"
	"github.com/vrnvgasu/gophprofile/internal/handler"
	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/repository/postgres"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
)

const shutdownTimeout = 5 * time.Second

func main() {
	cfg := parseConfig()

	if err := logger.Initialize(cfg.LogLevel); err != nil {
		log.Fatalf("logger.Initialize: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	storage := postgres.NewStorage()
	if err := storage.Start(ctx, cfg.DatabaseURI); err != nil {
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

	producer, err := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		logger.Log.Fatalf("kafka.NewProducer: %v", err)
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

	go func() {
		logger.Log.Infow("starting server", "address", cfg.RunAddress)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Log.Fatalf("srv.ListenAndServe: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Log.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.Errorw("srv.Shutdown", "error", err)
	}
}
