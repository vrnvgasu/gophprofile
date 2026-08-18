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
	cfg := parseConfig()

	if err := logger.Initialize(cfg.LogLevel, "server"); err != nil {
		log.Fatalf("logger.Initialize: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, serviceName, version, cfg.OTLPEndpoint, cfg.TraceSampleRate)
	if err != nil {
		logger.Fatalf("telemetry.Init: %v", err)
	}

	logger.AttachOTLP(serviceName)

	storage := postgres.NewStorage()
	if err := storage.Start(ctx, cfg.DatabaseURI); err != nil {
		logger.Fatalf("storage.Start: %v", err)
	}
	defer func() { _ = storage.Stop() }()

	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			logger.Log.Error("telemetry shutdown", "error", err)
		}
	}()

	if err := metrics.RegisterDBStats(storage.DB(), "gophprofile"); err != nil {
		logger.Fatalf("metrics.RegisterDBStats: %v", err)
	}

	if err := metrics.RegisterStats(storage); err != nil {
		logger.Fatalf("metrics.RegisterStats: %v", err)
	}

	objects, err := s3.NewStorage(ctx, s3.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		logger.Fatalf("s3.NewStorage: %v", err)
	}

	producer, err := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		logger.Fatalf("kafka.NewProducer: %v", err)
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
		logger.Log.Info("starting server", "address", cfg.RunAddress)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("srv.ListenAndServe: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Log.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.Error("srv.Shutdown", "error", err)
	}
}
