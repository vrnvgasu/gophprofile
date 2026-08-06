// Package repository - интерфейс хранилища метаданных аватарок.
package repository

import (
	"context"

	"github.com/vrnvgasu/gophprofile/internal/model"
)

//go:generate mockgen -destination=./mocks/mock.go -package=mocks . AvatarStorage
type AvatarStorage interface {
	CreateAvatar(ctx context.Context, a *model.Avatar) error
	GetAvatar(ctx context.Context, id string) (*model.Avatar, error)
	GetUserAvatars(ctx context.Context, userID string) ([]model.Avatar, error)
	GetLastUserAvatar(ctx context.Context, userID string) (*model.Avatar, error)
	SoftDeleteAvatar(ctx context.Context, id string) error
	SetProcessingStatus(ctx context.Context, id string, status model.ProcessingStatus) error
	SaveThumbnails(ctx context.Context, id string, thumbnails map[string]string) error
	Ping(ctx context.Context) error
	Stop() error
}
