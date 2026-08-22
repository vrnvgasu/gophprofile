package metrics

import (
	"context"
	"database/sql"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/model"
)

// statsProvider отдает сводку по аватаркам для бизнес-метрик.
type statsProvider interface {
	Stats(ctx context.Context) (*model.Stats, error)
}

// RegisterStats публикует бизнес-метрики по аватаркам.
func RegisterStats(provider statsProvider) error {
	storageBytes, err := meter.Int64ObservableGauge(
		"avatars_storage_bytes",
		metric.WithDescription("Total storage used by avatars"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return err
	}

	avatarsTotal, err := meter.Int64ObservableGauge(
		"avatars_total",
		metric.WithDescription("Number of avatars by processing status"),
	)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		stats, statsErr := provider.Stats(ctx)
		if statsErr != nil {
			logger.WithContext(ctx).Error("metrics.RegisterStats", "error", statsErr)

			return nil
		}

		observer.ObserveInt64(storageBytes, stats.TotalBytes)

		for status, count := range stats.ByProcessingStatus {
			observer.ObserveInt64(avatarsTotal, count,
				metric.WithAttributes(attribute.String("processing_status", status)))
		}

		return nil
	}, storageBytes, avatarsTotal)

	return err
}

// RegisterDBStats публикует состояние пула соединений с БД.
func RegisterDBStats(db *sql.DB, dbName string) error {
	_, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(attribute.String("db_name", dbName)),
	)

	return err
}
