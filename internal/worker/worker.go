// Package worker - обработчик событий об аватарках из брокера сообщений.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/metrics"
	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/pkg/breaker"
	"github.com/vrnvgasu/gophprofile/pkg/images"
	"github.com/vrnvgasu/gophprofile/pkg/retry"
)

// thumbnailSizes описывает миниатюры, которые создает воркер.
var thumbnailSizes = []struct {
	Name   string
	Width  int
	Height int
}{
	{Name: model.ThumbnailSmall, Width: 100, Height: 100},
	{Name: model.ThumbnailMedium, Width: 300, Height: 300},
}

//go:generate mockgen -destination=./mocks/mock.go -package=mocks . ObjectStorage
type ObjectStorage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type Worker struct {
	storage repository.AvatarStorage
	objects ObjectStorage
}

func New(storage repository.AvatarStorage, objects ObjectStorage) *Worker {
	return &Worker{storage: storage, objects: objects}
}

func (w *Worker) Handle(ctx context.Context, event model.Event) (err error) {
	start := time.Now()

	defer func() {
		metrics.RecordEventProcessed(ctx, string(event.Type), err, time.Since(start))
	}()

	switch event.Type {
	case model.EventAvatarUploaded:
		var payload model.AvatarUploadEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("worker.Handle Unmarshal upload: %w", err)
		}

		return w.HandleUploadEvent(ctx, payload)

	case model.EventAvatarDeleted:
		var payload model.AvatarDeleteEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("worker.Handle Unmarshal delete: %w", err)
		}

		return w.HandleDeleteEvent(ctx, payload)

	default:
		logger.WithContext(ctx).Warn("worker.Handle unknown event type", "type", event.Type, "event_id", event.ID)
		return nil
	}
}

// HandleUploadEvent создает миниатюры для загруженной аватарки.
func (w *Worker) HandleUploadEvent(ctx context.Context, event model.AvatarUploadEvent) error {
	avatarRecord, err := w.storage.GetAvatar(ctx, event.AvatarID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Аватарку успели удалить — обрабатывать нечего.
			logger.WithContext(ctx).Info("worker.HandleUploadEvent avatar gone", "avatar_id", event.AvatarID)
			return nil
		}

		return fmt.Errorf("worker.HandleUploadEvent GetAvatar: %w", err)
	}

	if avatarRecord.ProcessingStatus == model.ProcessingStatusCompleted {
		logger.WithContext(ctx).Info("worker.HandleUploadEvent already processed", "avatar_id", event.AvatarID)
		return nil
	}

	if err = w.storage.SetProcessingStatus(ctx, event.AvatarID, model.ProcessingStatusProcessing); err != nil {
		return fmt.Errorf("worker.HandleUploadEvent SetProcessingStatus: %w", err)
	}

	thumbnails, err := w.makeThumbnails(ctx, event.AvatarID, event.S3Key)
	if err != nil {
		if statusErr := w.storage.SetProcessingStatus(
			ctx, event.AvatarID, model.ProcessingStatusFailed,
		); statusErr != nil {
			logger.WithContext(ctx).Error("worker.HandleUploadEvent SetProcessingStatus failed",
				"avatar_id", event.AvatarID, "error", statusErr)
		}

		return err
	}

	if err = w.storage.SaveThumbnails(ctx, event.AvatarID, thumbnails); err != nil {
		return fmt.Errorf("worker.HandleUploadEvent SaveThumbnails: %w", err)
	}

	logger.WithContext(ctx).Info("worker.HandleUploadEvent done", "avatar_id", event.AvatarID)

	return nil
}

// HandleDeleteEvent удаляет файлы аватарки из объектного хранилища.
func (w *Worker) HandleDeleteEvent(ctx context.Context, event model.AvatarDeleteEvent) error {
	for _, key := range event.S3Keys {
		err := withRetry(func() error { return w.objects.Delete(ctx, key) })
		if err != nil && !errors.Is(err, s3.ErrNotFound) {
			return fmt.Errorf("worker.HandleDeleteEvent Delete %q: %w", key, err)
		}
	}

	logger.WithContext(ctx).Info("worker.HandleDeleteEvent done",
		"avatar_id", event.AvatarID, "keys", len(event.S3Keys))

	return nil
}

func (w *Worker) makeThumbnails(ctx context.Context, avatarID, s3Key string) (map[string]string, error) {
	original, err := w.download(ctx, s3Key)
	if err != nil {
		return nil, fmt.Errorf("worker.makeThumbnails download: %w", err)
	}

	thumbnails := make(map[string]string, len(thumbnailSizes))

	for _, size := range thumbnailSizes {
		data, thumbErr := images.Thumbnail(original, size.Width, size.Height)
		if thumbErr != nil {
			return nil, fmt.Errorf("worker.makeThumbnails %s: %w", size.Name, thumbErr)
		}

		key := avatar.ThumbnailKey(avatarID, size.Name)
		thumbErr = withRetry(func() error {
			return w.objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/jpeg")
		})
		if thumbErr != nil {
			return nil, fmt.Errorf("worker.makeThumbnails put %s: %w", size.Name, thumbErr)
		}

		thumbnails[size.Name] = key
	}

	return thumbnails, nil
}

func (w *Worker) download(ctx context.Context, key string) ([]byte, error) {
	var data []byte

	err := withRetry(func() error {
		body, err := w.objects.Get(ctx, key)
		if err != nil {
			return err
		}
		defer func() { _ = body.Close() }()

		data, err = io.ReadAll(body)

		return err
	})
	if err != nil {
		return nil, err
	}

	return data, nil
}

func withRetry(fn func() error) error {
	return retry.Retry(func() error {
		err := fn()
		if err == nil {
			return nil
		}

		if errors.Is(err, s3.ErrNotFound) || errors.Is(err, context.Canceled) || errors.Is(err, breaker.ErrOpen) {
			return err
		}

		return retry.NewRetryableError(err)
	})
}
