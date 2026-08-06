// Package s3 - объектное хранилище файлов поверх S3-совместимого API.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("s3: object not found")

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type Storage struct {
	client *minio.Client
	bucket string
}

func NewStorage(ctx context.Context, cfg Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("s3.NewStorage New: %w", err)
	}

	s := &Storage{client: client, bucket: cfg.Bucket}
	if err = s.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return s, nil
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

// Put сохраняет объект под указанным ключом.
func (s *Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("s3.Put %q: %w", key, err)
	}

	return nil
}

// Get возвращает содержимое объекта. Вызывающий обязан закрыть возвращенный поток.
func (s *Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("s3.Get %q: %w", key, err)
	}

	// GetObject ленивый: ошибки отсутствия объекта всплывают только при первом обращении.
	if _, err = obj.Stat(); err != nil {
		_ = obj.Close()

		if minio.ToErrorResponse(err).StatusCode == 404 {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("s3.Get Stat %q: %w", key, err)
	}

	return obj, nil
}

// Delete удаляет объект.
func (s *Storage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("s3.Delete %q: %w", key, err)
	}

	return nil
}

func (s *Storage) Ping(ctx context.Context) error {
	if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("s3.Ping: %w", err)
	}

	return nil
}
