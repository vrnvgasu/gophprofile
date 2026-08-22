// Package logger предоставляет глобальный структурированный логгер на основе slog.
package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/trace"
)

var Log *slog.Logger = slog.New(discardHandler{})

var (
	stdout  slog.Handler
	service string
)

func Initialize(level, serviceName string) error {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return err
	}

	stdout = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	service = serviceName
	Log = slog.New(stdout).With("service", serviceName)

	return nil
}

func AttachOTLP(scope string) {
	if stdout == nil {
		return
	}

	Log = slog.New(fanout{stdout, otelslog.NewHandler(scope)}).With("service", service)
}

func WithContext(ctx context.Context) *slog.Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return Log
	}

	return Log.With(
		"trace_id", spanCtx.TraceID().String(),
		"span_id", spanCtx.SpanID().String(),
	)
}

type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

func (f fanout) Handle(ctx context.Context, record slog.Record) error {
	for _, h := range f {
		if !h.Enabled(ctx, record.Level) {
			continue
		}

		if err := h.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}

	return nil
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make(fanout, len(f))
	for i, h := range f {
		next[i] = h.WithAttrs(attrs)
	}

	return next
}

func (f fanout) WithGroup(name string) slog.Handler {
	next := make(fanout, len(f))
	for i, h := range f {
		next[i] = h.WithGroup(name)
	}

	return next
}

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }
