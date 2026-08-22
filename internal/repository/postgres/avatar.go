package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
)

const selectColumns = `id, user_id, file_name, mime_type, size_bytes, s3_key, thumbnail_s3_keys,
       width, height, upload_status, processing_status, created_at, updated_at, deleted_at`

func (s *Storage) CreateAvatar(ctx context.Context, a *model.Avatar) error {
	thumbnails, err := json.Marshal(a.ThumbnailS3Keys)
	if err != nil {
		return fmt.Errorf("postgres.CreateAvatar Marshal: %w", err)
	}

	const query = `
		INSERT INTO avatars (id, user_id, file_name, mime_type, size_bytes, s3_key, thumbnail_s3_keys,
		                     width, height, upload_status, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	err = withRetry(func() error {
		return s.db.QueryRowContext(ctx, query,
			a.ID, a.UserID, a.FileName, a.MimeType, a.SizeBytes, a.S3Key, thumbnails,
			a.Width, a.Height, a.UploadStatus, a.ProcessingStatus,
		).Scan(&a.CreatedAt, &a.UpdatedAt)
	}, newPostgresErrorClassifier())
	if err != nil {
		return fmt.Errorf("postgres.CreateAvatar: %w", err)
	}

	return nil
}

func (s *Storage) GetAvatar(ctx context.Context, id string) (*model.Avatar, error) {
	query := `SELECT ` + selectColumns + ` FROM avatars WHERE id = $1 AND deleted_at IS NULL`

	var avatar *model.Avatar
	err := withRetry(func() error {
		var scanErr error
		avatar, scanErr = scanAvatar(s.db.QueryRowContext(ctx, query, id))
		return scanErr
	}, newPostgresErrorClassifier())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}

		return nil, fmt.Errorf("postgres.GetAvatar: %w", err)
	}

	return avatar, nil
}

func (s *Storage) GetUserAvatars(ctx context.Context, userID string) ([]model.Avatar, error) {
	query := `SELECT ` + selectColumns + `
		FROM avatars WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`

	var avatars []model.Avatar
	err := withRetry(func() error {
		rows, err := s.db.QueryContext(ctx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		avatars = avatars[:0]
		for rows.Next() {
			avatar, scanErr := scanAvatar(rows)
			if scanErr != nil {
				return scanErr
			}

			avatars = append(avatars, *avatar)
		}

		return rows.Err()
	}, newPostgresErrorClassifier())
	if err != nil {
		return nil, fmt.Errorf("postgres.GetUserAvatars: %w", err)
	}

	return avatars, nil
}

func (s *Storage) GetLastUserAvatar(ctx context.Context, userID string) (*model.Avatar, error) {
	query := `SELECT ` + selectColumns + `
		FROM avatars WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`

	var avatar *model.Avatar
	err := withRetry(func() error {
		var scanErr error
		avatar, scanErr = scanAvatar(s.db.QueryRowContext(ctx, query, userID))
		return scanErr
	}, newPostgresErrorClassifier())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}

		return nil, fmt.Errorf("postgres.GetLastUserAvatar: %w", err)
	}

	return avatar, nil
}

func (s *Storage) SoftDeleteAvatar(ctx context.Context, id string) error {
	const query = `UPDATE avatars SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`

	err := withRetry(func() error {
		res, err := s.db.ExecContext(ctx, query, id)
		if err != nil {
			return err
		}

		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return repository.ErrNotFound
		}

		return nil
	}, newPostgresErrorClassifier())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return err
		}

		return fmt.Errorf("postgres.SoftDeleteAvatar: %w", err)
	}

	return nil
}

func (s *Storage) SetProcessingStatus(ctx context.Context, id string, status model.ProcessingStatus) error {
	const query = `UPDATE avatars SET processing_status = $2, updated_at = now() WHERE id = $1`

	err := withRetry(func() error {
		_, err := s.db.ExecContext(ctx, query, id, status)
		return err
	}, newPostgresErrorClassifier())
	if err != nil {
		return fmt.Errorf("postgres.SetProcessingStatus: %w", err)
	}

	return nil
}

func (s *Storage) SaveThumbnails(ctx context.Context, id string, thumbnails map[string]string) error {
	raw, err := json.Marshal(thumbnails)
	if err != nil {
		return fmt.Errorf("postgres.SaveThumbnails Marshal: %w", err)
	}

	const query = `UPDATE avatars
		SET thumbnail_s3_keys = $2, processing_status = $3, updated_at = now()
		WHERE id = $1`

	err = withRetry(func() error {
		_, execErr := s.db.ExecContext(ctx, query, id, raw, model.ProcessingStatusCompleted)
		return execErr
	}, newPostgresErrorClassifier())
	if err != nil {
		return fmt.Errorf("postgres.SaveThumbnails: %w", err)
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAvatar(row scanner) (*model.Avatar, error) {
	var (
		avatar     model.Avatar
		thumbnails []byte
	)

	err := row.Scan(
		&avatar.ID, &avatar.UserID, &avatar.FileName, &avatar.MimeType, &avatar.SizeBytes,
		&avatar.S3Key, &thumbnails, &avatar.Width, &avatar.Height,
		&avatar.UploadStatus, &avatar.ProcessingStatus,
		&avatar.CreatedAt, &avatar.UpdatedAt, &avatar.DeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if len(thumbnails) > 0 {
		if err = json.Unmarshal(thumbnails, &avatar.ThumbnailS3Keys); err != nil {
			return nil, fmt.Errorf("scanAvatar Unmarshal: %w", err)
		}
	}
	if avatar.ThumbnailS3Keys == nil {
		avatar.ThumbnailS3Keys = map[string]string{}
	}

	return &avatar, nil
}

// Stats возвращает сводку по живым аватаркам.
func (s *Storage) Stats(ctx context.Context) (*model.Stats, error) {
	const query = `
		SELECT COALESCE(SUM(size_bytes), 0), processing_status, COUNT(*)
		FROM avatars
		WHERE deleted_at IS NULL
		GROUP BY processing_status`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres.Stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := &model.Stats{ByProcessingStatus: map[string]int64{}}

	for rows.Next() {
		var (
			bytes  int64
			status string
			count  int64
		)

		if err = rows.Scan(&bytes, &status, &count); err != nil {
			return nil, fmt.Errorf("postgres.Stats Scan: %w", err)
		}

		stats.TotalBytes += bytes
		stats.ByProcessingStatus[status] = count
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.Stats rows: %w", err)
	}

	return stats, nil
}
