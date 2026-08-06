package handler

import (
	"fmt"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/model"
)

type uploadResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// dimensions — размеры изображения в пикселях.
type dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// thumbnail — ссылка на миниатюру определенного размера.
type thumbnail struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}

type metadataResponse struct {
	ID         string      `json:"id"`
	UserID     string      `json:"user_id"`
	FileName   string      `json:"file_name"`
	MimeType   string      `json:"mime_type"`
	Size       int64       `json:"size"`
	Status     string      `json:"status"`
	URL        string      `json:"url"`
	Dimensions dimensions  `json:"dimensions"`
	Thumbnails []thumbnail `json:"thumbnails"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

func newUploadResponse(a *model.Avatar) uploadResponse {
	return uploadResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		URL:       avatarURL(a.ID),
		Status:    statusOf(a),
		CreatedAt: a.CreatedAt,
	}
}

func newMetadataResponse(a *model.Avatar) metadataResponse {
	thumbnails := make([]thumbnail, 0, len(a.ThumbnailS3Keys))
	for _, size := range []string{model.ThumbnailSmall, model.ThumbnailMedium} {
		if _, ok := a.ThumbnailKey(size); ok {
			thumbnails = append(thumbnails, thumbnail{Size: size, URL: thumbnailURL(a.ID, size)})
		}
	}

	return metadataResponse{
		ID:         a.ID,
		UserID:     a.UserID,
		FileName:   a.FileName,
		MimeType:   a.MimeType,
		Size:       a.SizeBytes,
		Status:     statusOf(a),
		URL:        avatarURL(a.ID),
		Dimensions: dimensions{Width: a.Width, Height: a.Height},
		Thumbnails: thumbnails,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

func newMetadataList(avatars []model.Avatar) []metadataResponse {
	list := make([]metadataResponse, 0, len(avatars))
	for i := range avatars {
		list = append(list, newMetadataResponse(&avatars[i]))
	}

	return list
}

func statusOf(a *model.Avatar) string {
	if a.ProcessingStatus == model.ProcessingStatusFailed {
		return "failed"
	}
	if a.ProcessingStatus == model.ProcessingStatusCompleted {
		return "completed"
	}

	return "processing"
}

func avatarURL(id string) string {
	return "/api/v1/avatars/" + id
}

func thumbnailURL(id, size string) string {
	return fmt.Sprintf("/api/v1/avatars/%s?size=%s", id, size)
}
