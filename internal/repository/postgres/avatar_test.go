package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
)

var avatarColumns = []string{
	"id", "user_id", "file_name", "mime_type", "size_bytes", "s3_key", "thumbnail_s3_keys",
	"width", "height", "upload_status", "processing_status", "created_at", "updated_at", "deleted_at",
}

func newStorageMock(t *testing.T) (*Storage, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})

	return &Storage{db: db}, mock
}

func avatarRow(id, userID string) *sqlmock.Rows {
	now := time.Now()

	return sqlmock.NewRows(avatarColumns).AddRow(
		id, userID, "photo.png", "image/png", int64(1024), "avatars/"+id+"/original.png",
		[]byte(`{"100x100":"thumbnails/`+id+`/100x100.jpg"}`),
		800, 600, model.UploadStatusUploaded, model.ProcessingStatusCompleted,
		now, now, nil,
	)
}

func TestCreateAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		now := time.Now()

		mock.ExpectQuery("INSERT INTO avatars").
			WithArgs("avatar-1", "user-1", "photo.png", "image/png", int64(1024),
				"avatars/avatar-1/original.png", []byte(`{}`), 800, 600,
				model.UploadStatusUploaded, model.ProcessingStatusPending).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))

		avatar := &model.Avatar{
			ID:               "avatar-1",
			UserID:           "user-1",
			FileName:         "photo.png",
			MimeType:         "image/png",
			SizeBytes:        1024,
			S3Key:            "avatars/avatar-1/original.png",
			ThumbnailS3Keys:  map[string]string{},
			Width:            800,
			Height:           600,
			UploadStatus:     model.UploadStatusUploaded,
			ProcessingStatus: model.ProcessingStatusPending,
		}

		require.NoError(t, storage.CreateAvatar(context.Background(), avatar))
		assert.False(t, avatar.CreatedAt.IsZero())
	})

	t.Run("Failure", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		// Постоянная ошибка PostgreSQL не должна повторяться.
		mock.ExpectQuery("INSERT INTO avatars").
			WillReturnError(&pgconn.PgError{Code: pgerrcode.UniqueViolation, Message: "duplicate key"})

		err := storage.CreateAvatar(context.Background(), &model.Avatar{ID: "avatar-1"})
		require.Error(t, err)
	})
}

func TestGetAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) FROM avatars WHERE id = ").
			WithArgs("avatar-1").
			WillReturnRows(avatarRow("avatar-1", "user-1"))

		found, err := storage.GetAvatar(context.Background(), "avatar-1")
		require.NoError(t, err)

		assert.Equal(t, "avatar-1", found.ID)
		assert.Equal(t, "user-1", found.UserID)
		assert.Equal(t, "thumbnails/avatar-1/100x100.jpg", found.ThumbnailS3Keys[model.ThumbnailSmall])
	})

	t.Run("Not found", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) FROM avatars WHERE id = ").
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := storage.GetAvatar(context.Background(), "missing")
		require.ErrorIs(t, err, repository.ErrNotFound)
	})
}

func TestGetUserAvatars(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		rows := avatarRow("avatar-2", "user-1")
		rows.AddRow(
			"avatar-1", "user-1", "old.png", "image/png", int64(512), "avatars/avatar-1/original.png",
			nil, 100, 100, model.UploadStatusUploaded, model.ProcessingStatusPending,
			time.Now(), time.Now(), nil,
		)

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) FROM avatars WHERE user_id = ").
			WithArgs("user-1").
			WillReturnRows(rows)

		avatars, err := storage.GetUserAvatars(context.Background(), "user-1")
		require.NoError(t, err)
		require.Len(t, avatars, 2)

		assert.Equal(t, "avatar-2", avatars[0].ID)
		// Пустой JSONB превращается в пустую карту, а не в nil.
		assert.NotNil(t, avatars[1].ThumbnailS3Keys)
	})

	t.Run("Empty list", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) FROM avatars WHERE user_id = ").
			WithArgs("user-2").
			WillReturnRows(sqlmock.NewRows(avatarColumns))

		avatars, err := storage.GetUserAvatars(context.Background(), "user-2")
		require.NoError(t, err)
		assert.Empty(t, avatars)
	})
}

func TestGetLastUserAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) ORDER BY created_at DESC LIMIT 1").
			WithArgs("user-1").
			WillReturnRows(avatarRow("avatar-1", "user-1"))

		found, err := storage.GetLastUserAvatar(context.Background(), "user-1")
		require.NoError(t, err)
		assert.Equal(t, "avatar-1", found.ID)
	})

	t.Run("User has no avatars", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectQuery("SELECT (.+) ORDER BY created_at DESC LIMIT 1").
			WithArgs("user-1").
			WillReturnError(sql.ErrNoRows)

		_, err := storage.GetLastUserAvatar(context.Background(), "user-1")
		require.ErrorIs(t, err, repository.ErrNotFound)
	})
}

func TestSoftDeleteAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectExec("UPDATE avatars SET deleted_at").
			WithArgs("avatar-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, storage.SoftDeleteAvatar(context.Background(), "avatar-1"))
	})

	t.Run("Already deleted", func(t *testing.T) {
		t.Parallel()

		storage, mock := newStorageMock(t)
		mock.ExpectExec("UPDATE avatars SET deleted_at").
			WithArgs("avatar-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := storage.SoftDeleteAvatar(context.Background(), "avatar-1")
		require.ErrorIs(t, err, repository.ErrNotFound)
	})
}

func TestSetProcessingStatus(t *testing.T) {
	t.Parallel()

	storage, mock := newStorageMock(t)
	mock.ExpectExec("UPDATE avatars SET processing_status").
		WithArgs("avatar-1", model.ProcessingStatusProcessing).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, storage.SetProcessingStatus(
		context.Background(), "avatar-1", model.ProcessingStatusProcessing))
}

func TestSaveThumbnails(t *testing.T) {
	t.Parallel()

	thumbnails := map[string]string{model.ThumbnailSmall: "thumbnails/avatar-1/100x100.jpg"}

	storage, mock := newStorageMock(t)
	mock.ExpectExec("UPDATE avatars").
		WithArgs("avatar-1", []byte(`{"100x100":"thumbnails/avatar-1/100x100.jpg"}`),
			model.ProcessingStatusCompleted).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, storage.SaveThumbnails(context.Background(), "avatar-1", thumbnails))
}

func TestPingAndStop(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	storage := &Storage{db: db}

	mock.ExpectPing()
	require.NoError(t, storage.Ping(context.Background()))

	mock.ExpectClose()
	require.NoError(t, storage.Stop())
	assert.NoError(t, mock.ExpectationsWereMet())
}
