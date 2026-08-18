package metrics

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/vrnvgasu/gophprofile/internal/model"
)

var reader = sdkmetric.NewManualReader()

func TestMain(m *testing.M) {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))

	os.Exit(m.Run())
}

func collect(t *testing.T) map[string]metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	result := make(map[string]metricdata.Metrics)

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			result[metric.Name] = metric
		}
	}

	return result
}

func sumFor(t *testing.T, metric metricdata.Metrics, attrs ...attribute.KeyValue) int64 {
	t.Helper()

	sum, ok := metric.Data.(metricdata.Sum[int64])
	require.True(t, ok, "ожидался Sum[int64] у %s", metric.Name)

	want := attribute.NewSet(attrs...)

	for _, point := range sum.DataPoints {
		if point.Attributes.Equals(&want) {
			return point.Value
		}
	}

	t.Fatalf("нет точки с атрибутами %v у %s", attrs, metric.Name)

	return 0
}

func TestStatus(t *testing.T) {
	assert.Equal(t, statusSuccess, status(nil))
	assert.Equal(t, statusError, status(errors.New("boom")))
}

func TestRecordHTTPRequest(t *testing.T) {
	ctx := context.Background()

	RecordHTTPRequest(ctx, http.MethodGet, "/api/v1/avatars/{avatar_id}", http.StatusOK, 150*time.Millisecond)

	collected := collect(t)

	total, ok := collected["http_requests_total"]
	require.True(t, ok, "метрика http_requests_total не опубликована")
	assert.Equal(t, int64(1), sumFor(t, total,
		attribute.String("method", http.MethodGet),
		attribute.String("route", "/api/v1/avatars/{avatar_id}"),
		attribute.String("status", "200"),
	))

	duration, ok := collected["http_request_duration_seconds"]
	require.True(t, ok, "метрика http_request_duration_seconds не опубликована")

	histogram, ok := duration.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.NotEmpty(t, histogram.DataPoints)
	assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)
}

func TestRecordBusinessEvents(t *testing.T) {
	ctx := context.Background()

	RecordUpload(ctx, nil, time.Second)
	RecordDelete(ctx, errors.New("boom"))
	RecordEventProcessed(ctx, "avatar.uploaded", nil, time.Second)
	RecordConsumerLag(ctx, "avatar-events", 0, 42)

	collected := collect(t)

	assert.Equal(t, int64(1), sumFor(t, collected["avatars_uploads_total"],
		attribute.String("status", statusSuccess)))
	assert.Equal(t, int64(1), sumFor(t, collected["avatars_deletes_total"],
		attribute.String("status", statusError)))
	assert.Equal(t, int64(1), sumFor(t, collected["avatar_events_processed_total"],
		attribute.String("type", "avatar.uploaded"),
		attribute.String("status", statusSuccess)))

	lag, ok := collected["avatar_consumer_lag"].Data.(metricdata.Gauge[int64])
	require.True(t, ok, "avatar_consumer_lag должен быть Gauge")
	require.NotEmpty(t, lag.DataPoints)
	assert.Equal(t, int64(42), lag.DataPoints[0].Value)
}

func TestUptimePublished(t *testing.T) {
	uptime, ok := collect(t)["service_uptime_seconds"]
	require.True(t, ok, "метрика service_uptime_seconds не опубликована")

	gauge, ok := uptime.Data.(metricdata.Gauge[float64])
	require.True(t, ok)
	require.NotEmpty(t, gauge.DataPoints)
	assert.Positive(t, gauge.DataPoints[0].Value)
}

type stubProvider struct {
	stats *model.Stats
	err   error
}

func (s *stubProvider) Stats(context.Context) (*model.Stats, error) {
	return s.stats, s.err
}

func TestRegisterStats(t *testing.T) {
	t.Run("метрики заполняются из сводки", func(t *testing.T) {
		require.NoError(t, RegisterStats(&stubProvider{
			stats: &model.Stats{
				TotalBytes:         2048,
				ByProcessingStatus: map[string]int64{"completed": 3, "pending": 1},
			},
		}))

		collected := collect(t)

		storage, ok := collected["avatars_storage_bytes"].Data.(metricdata.Gauge[int64])
		require.True(t, ok)
		require.NotEmpty(t, storage.DataPoints)
		assert.Equal(t, int64(2048), storage.DataPoints[0].Value)

		avatars, ok := collected["avatars_total"].Data.(metricdata.Gauge[int64])
		require.True(t, ok)
		assert.Len(t, avatars.DataPoints, 2)
	})

	t.Run("ошибка провайдера не срывает сбор остальных метрик", func(t *testing.T) {
		require.NoError(t, RegisterStats(&stubProvider{err: errors.New("db is down")}))
		assert.Contains(t, collect(t), "http_requests_total")
	})
}
