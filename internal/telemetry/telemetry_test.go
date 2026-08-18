package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestInitWithoutEndpoint(t *testing.T) {
	shutdown, err := Init(context.Background(), "test", "1.0.0", "", 1.0)
	require.NoError(t, err)

	t.Run("телеметрия выключена, но приложение работает", func(t *testing.T) {
		_, span := otel.Tracer("test").Start(context.Background(), "op")
		defer span.End()

		// Без экспортера спан не записывается, но и не падает.
		assert.False(t, span.IsRecording())
	})

	t.Run("пропагатор настроен даже без экспортера", func(t *testing.T) {
		// Без этого трейс-контекст не уехал бы в заголовки Kafka.
		fields := otel.GetTextMapPropagator().Fields()
		assert.Contains(t, fields, "traceparent")
	})

	t.Run("shutdown безопасен", func(t *testing.T) {
		assert.NoError(t, shutdown(context.Background()))
	})
}

func TestPropagatorRoundTrip(t *testing.T) {
	setPropagator()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(
		trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled},
	))

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	restored := trace.SpanContextFromContext(
		otel.GetTextMapPropagator().Extract(context.Background(), carrier),
	)

	assert.Equal(t, traceID, restored.TraceID())
	assert.Equal(t, spanID, restored.SpanID())
}
