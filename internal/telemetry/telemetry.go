// Package telemetry настраивает OpenTelemetry.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellog "go.opentelemetry.io/otel/log/global"
	lognoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	shutdownTimeout = 5 * time.Second
	exportInterval  = 15 * time.Second
)

type ShutdownFunc func(context.Context) error

func Init(ctx context.Context, service, version, endpoint string, sampleRatio float64) (ShutdownFunc, error) {
	setPropagator()

	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otellog.SetLoggerProvider(lognoop.NewLoggerProvider())

		return func(context.Context) error { return nil }, nil
	}

	res, err := newResource(service, version)
	if err != nil {
		return nil, err
	}

	shutdownTraces, err := initTraces(ctx, res, endpoint, sampleRatio)
	if err != nil {
		return nil, err
	}

	shutdownMetrics, err := initMetrics(ctx, res, endpoint)
	if err != nil {
		return nil, err
	}

	shutdownLogs, err := initLogs(ctx, res, endpoint)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		return errors.Join(shutdownTraces(ctx), shutdownMetrics(ctx), shutdownLogs(ctx))
	}, nil
}

func newResource(service, version string) (*resource.Resource, error) {
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName(service),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return nil, fmt.Errorf("telemetry.newResource: %w", err)
	}

	return res, nil
}

func initTraces(ctx context.Context, res *resource.Resource, endpoint string, sampleRatio float64) (ShutdownFunc, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry.initTraces: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))),
	)

	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

func initMetrics(ctx context.Context, res *resource.Resource, endpoint string) (ShutdownFunc, error) {
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry.initMetrics: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(exportInterval),
		)),
	)

	otel.SetMeterProvider(provider)

	// Метрики рантайма (горутины, память, GC) — замена go_* из клиента Prometheus.
	if err := runtime.Start(runtime.WithMeterProvider(provider)); err != nil {
		return nil, fmt.Errorf("telemetry.initMetrics runtime: %w", err)
	}

	return provider.Shutdown, nil
}

func initLogs(ctx context.Context, res *resource.Resource, endpoint string) (ShutdownFunc, error) {
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry.initLogs: %w", err)
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	otellog.SetLoggerProvider(provider)

	return provider.Shutdown, nil
}

func setPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}
