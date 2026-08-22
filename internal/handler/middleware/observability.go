package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/vrnvgasu/gophprofile/internal/metrics"
)

func Observability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		ctx := r.Context()

		metrics.AddInFlight(ctx, 1)
		defer metrics.AddInFlight(ctx, -1)

		next.ServeHTTP(rw, r)

		route := routePattern(r)

		metrics.RecordHTTPRequest(ctx, r.Method, route, rw.status, time.Since(start))

		if span := trace.SpanFromContext(r.Context()); span.IsRecording() {
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				semconv.HTTPRoute(route),
				semconv.HTTPResponseStatusCode(rw.status),
			)
		}
	})
}

func routePattern(r *http.Request) string {
	if ctx := chi.RouteContext(r.Context()); ctx != nil {
		if pattern := ctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}

	return "unknown"
}
