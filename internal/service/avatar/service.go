// Package avatar реализует бизнес-логику загрузки, выдачи и удаления аватарок.
package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/google/uuid"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/pkg/images"
)

//go:generate mockgen -destination=./mocks/mock.go -package=mocks . ObjectStorage,Producer
type ObjectStorage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

type Producer interface {
	Publish(ctx context.Context, key string, event model.Event) error
	Ping(ctx context.Context) error
}

type Service struct {
	storage       repository.AvatarStorage
	objects       ObjectStorage
	producer      Producer
	maxUploadSize int64
}

func NewService(
	storage repository.AvatarStorage,
	objects ObjectStorage,
	producer Producer,
	maxUploadSize int64,
) *Service {
	return &Service{
		storage:       storage,
		objects:       objects,
		producer:      producer,
		maxUploadSize: maxUploadSize,
	}
}

type Content struct {
	Body     io.ReadCloser
	MimeType string
	ETag     string
}

func (s *Service) Upload(ctx context.Context, userID, fileName string, data []byte) (*model.Avatar, error) {
	if userID == "" {
		return nil, UnauthorizedError()
	}
	if len(data) == 0 {
		return nil, BadRequestError("Empty file", "File content is required")
	}
	if int64(len(data)) > s.maxUploadSize {
		return nil, TooLargeError(s.maxUploadSize)
	}

	info, err := images.Detect(data)
	if err != nil {
		if errors.Is(err, images.ErrUnsupportedFormat) {
			return nil, UnsupportedFormatError()
		}

		return nil, fmt.Errorf("avatar.Upload Detect: %w", err)
	}

	id := uuid.NewString()
	key := originalKey(id, info.Extension)

	if err = s.objects.Put(ctx, key, bytes.NewReader(data), int64(len(data)), info.MimeType); err != nil {
		return nil, fmt.Errorf("avatar.Upload Put: %w", err)
	}

	avatar := &model.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         sanitizeFileName(fileName, info.Extension),
		MimeType:         info.MimeType,
		SizeBytes:        int64(len(data)),
		S3Key:            key,
		ThumbnailS3Keys:  map[string]string{},
		Width:            info.Width,
		Height:           info.Height,
		UploadStatus:     model.UploadStatusUploaded,
		ProcessingStatus: model.ProcessingStatusPending,
	}

	if err = s.storage.CreateAvatar(ctx, avatar); err != nil {
		// Файл уже в хранилище, а записи о нем нет — убираем мусор.
		if delErr := s.objects.Delete(ctx, key); delErr != nil {
			return nil, fmt.Errorf("avatar.Upload CreateAvatar: %w (cleanup: %w)", err, delErr)
		}

		return nil, fmt.Errorf("avatar.Upload CreateAvatar: %w", err)
	}

	if err = s.publishUploadEvent(ctx, avatar); err != nil {
		// Событие не ушло — аватарка останется без миниатюр, но оригинал доступен.
		return nil, fmt.Errorf("avatar.Upload publish: %w", err)
	}

	return avatar, nil
}

// Metadata возвращает метаданные аватарки.
func (s *Service) Metadata(ctx context.Context, id string) (*model.Avatar, error) {
	if err := validateAvatarID(id); err != nil {
		return nil, err
	}

	avatar, err := s.storage.GetAvatar(ctx, id)
	if err != nil {
		return nil, wrapNotFound("avatar.Metadata", err)
	}

	return avatar, nil
}

// ListUserAvatars возвращает все активные аватарки пользователя, новые первыми.
func (s *Service) ListUserAvatars(ctx context.Context, userID string) ([]model.Avatar, error) {
	avatars, err := s.storage.GetUserAvatars(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("avatar.ListUserAvatars: %w", err)
	}

	return avatars, nil
}

// Download возвращает содержимое аватарки указанного размера.
// Если миниатюра запрошенного размера еще не создана, отдается оригинал.
func (s *Service) Download(ctx context.Context, id, size string) (*Content, error) {
	if err := validateAvatarID(id); err != nil {
		return nil, err
	}

	avatar, err := s.storage.GetAvatar(ctx, id)
	if err != nil {
		return nil, wrapNotFound("avatar.Download", err)
	}

	return s.content(ctx, avatar, size)
}

// DownloadUserAvatar возвращает содержимое последней загруженной аватарки пользователя.
func (s *Service) DownloadUserAvatar(ctx context.Context, userID, size string) (*Content, error) {
	avatar, err := s.storage.GetLastUserAvatar(ctx, userID)
	if err != nil {
		return nil, wrapNotFound("avatar.DownloadUserAvatar", err)
	}

	return s.content(ctx, avatar, size)
}

// Delete мягко удаляет аватарку и публикует событие на удаление файлов из хранилища.
func (s *Service) Delete(ctx context.Context, id, userID string) error {
	if userID == "" {
		return UnauthorizedError()
	}

	if err := validateAvatarID(id); err != nil {
		return err
	}

	avatar, err := s.storage.GetAvatar(ctx, id)
	if err != nil {
		return wrapNotFound("avatar.Delete", err)
	}

	if avatar.UserID != userID {
		return ForbiddenError()
	}

	if err = s.storage.SoftDeleteAvatar(ctx, id); err != nil {
		return wrapNotFound("avatar.Delete SoftDelete", err)
	}

	return s.publishDeleteEvent(ctx, avatar)
}

// DeleteUserAvatar мягко удаляет последнюю загруженную аватарку пользователя.
func (s *Service) DeleteUserAvatar(ctx context.Context, userID, requesterID string) error {
	if requesterID == "" {
		return UnauthorizedError()
	}
	if userID != requesterID {
		return ForbiddenError()
	}

	avatar, err := s.storage.GetLastUserAvatar(ctx, userID)
	if err != nil {
		return wrapNotFound("avatar.DeleteUserAvatar", err)
	}

	return s.Delete(ctx, avatar.ID, requesterID)
}

func (s *Service) content(ctx context.Context, avatar *model.Avatar, size string) (*Content, error) {
	key := avatar.S3Key
	mimeType := avatar.MimeType
	etag := avatar.ID + "-" + model.SizeOriginal

	if size != "" && size != model.SizeOriginal {
		if thumbKey, ok := avatar.ThumbnailKey(size); ok {
			key = thumbKey
			// Миниатюры воркер всегда сохраняет в JPEG.
			mimeType = "image/jpeg"
			etag = avatar.ID + "-" + size
		}
	}

	body, err := s.objects.Get(ctx, key)
	if err != nil {
		return nil, wrapNotFound("avatar.content", err)
	}

	return &Content{Body: body, MimeType: mimeType, ETag: etag}, nil
}

func (s *Service) publishUploadEvent(ctx context.Context, avatar *model.Avatar) error {
	event, err := model.NewEvent(uuid.NewString(), model.EventAvatarUploaded, model.AvatarUploadEvent{
		AvatarID: avatar.ID,
		UserID:   avatar.UserID,
		S3Key:    avatar.S3Key,
	})
	if err != nil {
		return fmt.Errorf("avatar.publishUploadEvent NewEvent: %w", err)
	}

	if err = s.producer.Publish(ctx, avatar.ID, event); err != nil {
		return fmt.Errorf("avatar.publishUploadEvent Publish: %w", err)
	}

	return nil
}

func (s *Service) publishDeleteEvent(ctx context.Context, avatar *model.Avatar) error {
	event, err := model.NewEvent(uuid.NewString(), model.EventAvatarDeleted, model.AvatarDeleteEvent{
		AvatarID: avatar.ID,
		S3Keys:   avatar.AllS3Keys(),
	})
	if err != nil {
		return fmt.Errorf("avatar.publishDeleteEvent NewEvent: %w", err)
	}

	if err = s.producer.Publish(ctx, avatar.ID, event); err != nil {
		return fmt.Errorf("avatar.publishDeleteEvent Publish: %w", err)
	}

	return nil
}

func originalKey(avatarID, ext string) string {
	return path.Join("avatars", avatarID, "original"+ext)
}

func ThumbnailKey(avatarID, size string) string {
	return path.Join("thumbnails", avatarID, size+".jpg")
}

func sanitizeFileName(fileName, ext string) string {
	name := path.Base(path.Clean(fileName))
	if name == "." || name == "/" || name == "" {
		return "avatar" + ext
	}

	return name
}

func validateAvatarID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return NotFoundError()
	}

	return nil
}

func wrapNotFound(op string, err error) error {
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, s3.ErrNotFound) {
		return NotFoundError()
	}

	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return err
	}

	return fmt.Errorf("%s: %w", op, err)
}
