// Package model содержит основные модели.
package model

import "time"

type UploadStatus string

const UploadStatusUploaded UploadStatus = "uploaded"

// ProcessingStatus - состояние асинхронной обработки аватарки.
type ProcessingStatus string

const (
	ProcessingStatusPending    ProcessingStatus = "pending"
	ProcessingStatusProcessing ProcessingStatus = "processing"
	ProcessingStatusCompleted  ProcessingStatus = "completed"
	ProcessingStatusFailed     ProcessingStatus = "failed"
)

const (
	ThumbnailSmall  = "100x100"
	ThumbnailMedium = "300x300"
	SizeOriginal    = "original"
)

type Avatar struct {
	ID               string
	UserID           string
	FileName         string
	MimeType         string
	SizeBytes        int64
	S3Key            string
	ThumbnailS3Keys  map[string]string
	Width            int
	Height           int
	UploadStatus     UploadStatus
	ProcessingStatus ProcessingStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// ThumbnailKey возвращает ключ миниатюры запрошенного размера.
func (a *Avatar) ThumbnailKey(size string) (string, bool) {
	key, ok := a.ThumbnailS3Keys[size]
	return key, ok
}

// AllS3Keys возвращает ключи всех файлов аватарки: оригинал и миниатюры.
func (a *Avatar) AllS3Keys() []string {
	keys := make([]string, 0, len(a.ThumbnailS3Keys)+1)
	keys = append(keys, a.S3Key)

	for _, size := range []string{ThumbnailSmall, ThumbnailMedium} {
		if key, ok := a.ThumbnailS3Keys[size]; ok {
			keys = append(keys, key)
		}
	}

	return keys
}

type Stats struct {
	TotalBytes         int64
	ByProcessingStatus map[string]int64
}
