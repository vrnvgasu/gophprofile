// Package metrics публикует метрики приложения через OpenTelemetry.
package metrics

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const scopeName = "github.com/vrnvgasu/gophprofile"

const (
	statusSuccess = "success"
	statusError   = "error"
)

var secondsBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var meter = otel.Meter(scopeName)

var (
	httpRequestsTotal = must(meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	))

	httpRequestDuration = must(meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(secondsBuckets...),
	))

	httpRequestsInFlight = must(meter.Int64UpDownCounter(
		"http_requests_in_flight",
		metric.WithDescription("Number of HTTP requests currently being served"),
	))
)

var (
	uploadsTotal = must(meter.Int64Counter(
		"avatars_uploads_total",
		metric.WithDescription("Total number of avatar uploads"),
	))

	uploadDuration = must(meter.Float64Histogram(
		"avatars_upload_duration_seconds",
		metric.WithDescription("Avatar upload duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(secondsBuckets...),
	))

	deletesTotal = must(meter.Int64Counter(
		"avatars_deletes_total",
		metric.WithDescription("Total number of avatar deletions"),
	))
)

var (
	eventsPublishedTotal = must(meter.Int64Counter(
		"avatar_events_published_total",
		metric.WithDescription("Total number of events published to the broker"),
	))

	eventsProcessedTotal = must(meter.Int64Counter(
		"avatar_events_processed_total",
		metric.WithDescription("Total number of events processed by the worker"),
	))

	eventProcessingDuration = must(meter.Float64Histogram(
		"avatar_event_processing_duration_seconds",
		metric.WithDescription("Event processing duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(secondsBuckets...),
	))

	consumerLag = must(meter.Int64Gauge(
		"avatar_consumer_lag",
		metric.WithDescription("Consumer group lag by partition"),
	))
)

var startedAt atomic.Int64

var _ = must(meter.Float64ObservableGauge(
	"service_uptime_seconds",
	metric.WithDescription("Seconds since service start"),
	metric.WithUnit("s"),
	metric.WithFloat64Callback(func(_ context.Context, observer metric.Float64Observer) error {
		started := startedAt.Load()
		if started == 0 {
			return nil
		}

		observer.Observe(time.Since(time.Unix(0, started)).Seconds())

		return nil
	}),
))

// MarkStarted фиксирует момент, с которого считается service_uptime_seconds.
// Вызывается, когда сервис готов принимать нагрузку.
func MarkStarted() {
	startedAt.Store(time.Now().UnixNano())
}

// RecordHTTPRequest снимает RED-метрики завершенного запроса.
func RecordHTTPRequest(ctx context.Context, method, route string, statusCode int, duration time.Duration) {
	httpRequestsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("route", route),
		attribute.String("status", strconv.Itoa(statusCode)),
	))

	httpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("route", route),
	))
}

// AddInFlight меняет счетчик запросов в обработке: +1 на входе, -1 на выходе.
func AddInFlight(ctx context.Context, delta int64) {
	httpRequestsInFlight.Add(ctx, delta)
}

// RecordUpload снимает метрики загрузки аватарки.
func RecordUpload(ctx context.Context, err error, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("status", status(err)))

	uploadsTotal.Add(ctx, 1, attrs)
	uploadDuration.Record(ctx, duration.Seconds(), attrs)
}

// RecordDelete считает удаления аватарок.
func RecordDelete(ctx context.Context, err error) {
	deletesTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status(err))))
}

// RecordEventPublished считает события, отправленные в брокер.
func RecordEventPublished(ctx context.Context, eventType string, err error) {
	eventsPublishedTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", eventType),
		attribute.String("status", status(err)),
	))
}

// RecordEventProcessed снимает метрики обработки события воркером.
func RecordEventProcessed(ctx context.Context, eventType string, err error, duration time.Duration) {
	eventsProcessedTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", eventType),
		attribute.String("status", status(err)),
	))

	eventProcessingDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("type", eventType),
	))
}

// RecordConsumerLag публикует лаг одной партиции.
func RecordConsumerLag(ctx context.Context, topic string, partition int32, lag int64) {
	consumerLag.Record(ctx, lag, metric.WithAttributes(
		attribute.String("topic", topic),
		attribute.String("partition", strconv.Itoa(int(partition))),
	))
}

// status превращает ошибку в значение атрибута status.
func status(err error) string {
	if err != nil {
		return statusError
	}

	return statusSuccess
}

// must разворачивает создание инструмента: ошибка здесь означает недопустимое
// имя метрики, то есть опечатку в коде, а не сбой в рантайме.
func must[T any](instrument T, err error) T {
	if err != nil {
		panic(err)
	}

	return instrument
}
