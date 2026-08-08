// Package handler содержит HTTP-обработчики сервиса аватарок.
package handler

import (
	"context"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

//go:generate mockgen -destination=./mocks/mock.go -package=mocks . App
type App interface {
	Upload(ctx context.Context, userID, fileName string, data []byte) (*model.Avatar, error)
	Metadata(ctx context.Context, id string) (*model.Avatar, error)
	ListUserAvatars(ctx context.Context, userID string) ([]model.Avatar, error)
	Download(ctx context.Context, id, size string) (*avatar.Content, error)
	DownloadUserAvatar(ctx context.Context, userID, size string) (*avatar.Content, error)
	Delete(ctx context.Context, id, userID string) error
	DeleteUserAvatar(ctx context.Context, userID, requesterID string) error
	Check(ctx context.Context) avatar.Health
}

type Handler struct {
	app           App
	maxUploadSize int64
}

func NewHandler(app App, maxUploadSize int64) *Handler {
	return &Handler{
		app:           app,
		maxUploadSize: maxUploadSize,
	}
}
