// Package s3 - объектное хранилище файлов поверх S3-совместимого API.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/vrnvgasu/gophprofile/pkg/breaker"
)

var ErrNotFound = errors.New("s3: object not found")

const (
	dialTimeout       = 2 * time.Second
	responseTimeout   = 5 * time.Second
	storageMaxRetries = 2
)

const (
	breakerFailures    = 3
	breakerOpenTimeout = 30 * time.Second
)

var tracer = otel.Tracer("gophprofile/s3")

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type Storage struct {
	client  *minio.Client
	bucket  string
	breaker *breaker.Breaker
}

func NewStorage(ctx context.Context, cfg Config) (*Storage, error) {
	transport, err := minio.DefaultTransport(cfg.UseSSL)
	if err != nil {
		return nil, fmt.Errorf("s3.NewStorage DefaultTransport: %w", err)
	}

	transport.DialContext = (&net.Dialer{Timeout: dialTimeout}).DialContext
	transport.ResponseHeaderTimeout = responseTimeout

	minio.MaxRetry = storageMaxRetries

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.UseSSL,
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("s3.NewStorage New: %w", err)
	}

	s := &Storage{client: client, bucket: cfg.Bucket, breaker: newBreaker()}
	if err = s.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func newBreaker() *breaker.Breaker {
	cnf := breaker.DefaultConfig()
	cnf.FailureThreshold = breakerFailures
	cnf.OpenTimeout = breakerOpenTimeout
	cnf.IsFailure = func(err error) bool {
		return err != nil && !errors.Is(err, ErrNotFound)
	}

	return breaker.NewWithSettings("s3", cnf)
}

func (s *Storage) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("s3.ensureBucket BucketExists: %w", err)
	}
	if exists {
		return nil
	}

	if err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("s3.ensureBucket MakeBucket: %w", err)
	}

	return nil
}

func (s *Storage) startSpan(ctx context.Context, op, key string) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, "s3."+op, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		semconv.RPCSystemKey.String("s3"),
		attribute.String("s3.operation", op),
		attribute.String("s3.bucket", s.bucket),
		attribute.String("s3.key", key),
	)

	return ctx, span
}

func finishSpan(span trace.Span, err error) {
	if err != nil && !errors.Is(err, ErrNotFound) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	span.End()
}

// Put сохраняет объект под указанным ключом.
func (s *Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (err error) {
	ctx, span := s.startSpan(ctx, "put", key)
	span.SetAttributes(attribute.Int64("s3.size", size))

	defer func() { finishSpan(span, err) }()

	return s.breaker.Do(ctx, func() error {
		if _, putErr := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
			ContentType: contentType,
		}); putErr != nil {
			return fmt.Errorf("s3.Put %q: %w", key, putErr)
		}

		return nil
	})
}

// Get возвращает содержимое объекта. Вызывающий обязан закрыть возвращенный поток.
func (s *Storage) Get(ctx context.Context, key string) (_ io.ReadCloser, err error) {
	ctx, span := s.startSpan(ctx, "get", key)
	defer func() { finishSpan(span, err) }()

	var obj *minio.Object

	err = s.breaker.Do(ctx, func() error {
		object, getErr := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
		if getErr != nil {
			return fmt.Errorf("s3.Get %q: %w", key, getErr)
		}

		// GetObject ленивый: ошибки отсутствия объекта всплывают только при первом обращении.
		if _, statErr := object.Stat(); statErr != nil {
			_ = object.Close()

			if minio.ToErrorResponse(statErr).StatusCode == 404 {
				return ErrNotFound
			}

			return fmt.Errorf("s3.Get Stat %q: %w", key, statErr)
		}

		obj = object

		return nil
	})
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// Delete удаляет объект.
func (s *Storage) Delete(ctx context.Context, key string) (err error) {
	ctx, span := s.startSpan(ctx, "delete", key)
	defer func() { finishSpan(span, err) }()

	return s.breaker.Do(ctx, func() error {
		if delErr := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); delErr != nil {
			return fmt.Errorf("s3.Delete %q: %w", key, delErr)
		}

		return nil
	})
}

func (s *Storage) Ping(ctx context.Context) error {
	if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("s3.Ping: %w", err)
	}

	return nil
}
