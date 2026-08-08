package avatar

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
	repomocks "github.com/vrnvgasu/gophprofile/internal/repository/mocks"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar/mocks"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
)

const maxUploadSize = 10 << 20

const (
	testAvatarID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testMissingID = "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

type deps struct {
	storage  *repomocks.MockAvatarStorage
	objects  *mocks.MockObjectStorage
	producer *mocks.MockProducer
	service  *Service
}

func newDeps(t *testing.T, maxSize int64) *deps {
	t.Helper()

	ctrl := gomock.NewController(t)
	storage := repomocks.NewMockAvatarStorage(ctrl)
	objects := mocks.NewMockObjectStorage(ctrl)
	producer := mocks.NewMockProducer(ctrl)

	return &deps{
		storage:  storage,
		objects:  objects,
		producer: producer,
		service:  NewService(storage, objects, producer, maxSize),
	}
}

func TestUpload(t *testing.T) {
	t.Parallel()

	image := testPNG(t, 20, 10)

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.objects.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any(), int64(len(image)), "image/png").
			Return(nil)
		d.storage.EXPECT().CreateAvatar(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, a *model.Avatar) error {
				a.CreatedAt = time.Now()
				return nil
			})
		d.producer.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, key string, event model.Event) error {
				assert.Equal(t, model.EventAvatarUploaded, event.Type)
				assert.NotEmpty(t, key)
				return nil
			})

		created, err := d.service.Upload(context.Background(), "user-1", "photo.png", image)
		require.NoError(t, err)

		assert.NotEmpty(t, created.ID)
		assert.Equal(t, "user-1", created.UserID)
		assert.Equal(t, "photo.png", created.FileName)
		assert.Equal(t, "image/png", created.MimeType)
		assert.Equal(t, 20, created.Width)
		assert.Equal(t, 10, created.Height)
		assert.Equal(t, model.ProcessingStatusPending, created.ProcessingStatus)
		assert.Contains(t, created.S3Key, created.ID)
	})

	t.Run("Empty user id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		_, err := d.service.Upload(context.Background(), "", "photo.png", image)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("Empty file", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		_, err := d.service.Upload(context.Background(), "user-1", "photo.png", nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("File too large", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, 10)

		_, err := d.service.Upload(context.Background(), "user-1", "photo.png", image)
		assert.ErrorIs(t, err, ErrTooLarge)
	})

	t.Run("Unsupported format", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		_, err := d.service.Upload(context.Background(), "user-1", "notes.txt", []byte("plain text"))
		assert.ErrorIs(t, err, ErrUnsupportedFormat)
	})

	t.Run("Storage failure removes uploaded object", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.objects.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)
		d.storage.EXPECT().CreateAvatar(gomock.Any(), gomock.Any()).Return(errors.New("db is down"))
		d.objects.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)

		_, err := d.service.Upload(context.Background(), "user-1", "photo.png", image)
		require.Error(t, err)

		var serviceErr *ServiceError
		assert.False(t, errors.As(err, &serviceErr), "ошибка базы не должна превращаться в ошибку клиента")
	})

	t.Run("Default file name", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.objects.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)
		d.storage.EXPECT().CreateAvatar(gomock.Any(), gomock.Any()).Return(nil)
		d.producer.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		created, err := d.service.Upload(context.Background(), "user-1", "", image)
		require.NoError(t, err)
		assert.Equal(t, "avatar.png", created.FileName)
	})
}

func TestDownload(t *testing.T) {
	t.Parallel()

	stored := &model.Avatar{
		ID:       testAvatarID,
		UserID:   "user-1",
		MimeType: "image/png",
		S3Key:    "avatars/avatar-1/original.png",
		ThumbnailS3Keys: map[string]string{
			model.ThumbnailSmall: "thumbnails/avatar-1/100x100.jpg",
		},
	}

	tests := []struct {
		name             string
		size             string
		expectedKey      string
		expectedMimeType string
		expectedETag     string
	}{
		{
			name:             "Original",
			size:             "",
			expectedKey:      stored.S3Key,
			expectedMimeType: "image/png",
			expectedETag:     testAvatarID + "-original",
		},
		{
			name:             "Existing thumbnail",
			size:             model.ThumbnailSmall,
			expectedKey:      "thumbnails/avatar-1/100x100.jpg",
			expectedMimeType: "image/jpeg",
			expectedETag:     testAvatarID + "-100x100",
		},
		{
			name:             "Thumbnail is not ready yet",
			size:             model.ThumbnailMedium,
			expectedKey:      stored.S3Key,
			expectedMimeType: "image/png",
			expectedETag:     testAvatarID + "-original",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := newDeps(t, maxUploadSize)
			d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)
			d.objects.EXPECT().Get(gomock.Any(), tt.expectedKey).
				Return(io.NopCloser(bytes.NewReader([]byte("data"))), nil)

			content, err := d.service.Download(context.Background(), testAvatarID, tt.size)
			require.NoError(t, err)
			defer func() { _ = content.Body.Close() }()

			assert.Equal(t, tt.expectedMimeType, content.MimeType)
			assert.Equal(t, tt.expectedETag, content.ETag)
		})
	}

	t.Run("Avatar not found", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testMissingID).Return(nil, repository.ErrNotFound)

		_, err := d.service.Download(context.Background(), testMissingID, "")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Malformed id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		_, err := d.service.Download(context.Background(), "not-a-uuid", "")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Object is missing in storage", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)
		d.objects.EXPECT().Get(gomock.Any(), stored.S3Key).Return(nil, s3.ErrNotFound)

		_, err := d.service.Download(context.Background(), testAvatarID, "")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestDownloadUserAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		stored := &model.Avatar{ID: testAvatarID, MimeType: "image/png", S3Key: "avatars/avatar-1/original.png"}

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetLastUserAvatar(gomock.Any(), "user-1").Return(stored, nil)
		d.objects.EXPECT().Get(gomock.Any(), stored.S3Key).
			Return(io.NopCloser(bytes.NewReader([]byte("data"))), nil)

		content, err := d.service.DownloadUserAvatar(context.Background(), "user-1", "")
		require.NoError(t, err)
		defer func() { _ = content.Body.Close() }()

		assert.Equal(t, "image/png", content.MimeType)
	})

	t.Run("User has no avatars", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetLastUserAvatar(gomock.Any(), "user-1").Return(nil, repository.ErrNotFound)

		_, err := d.service.DownloadUserAvatar(context.Background(), "user-1", "")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestMetadataAndList(t *testing.T) {
	t.Parallel()

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()

		stored := &model.Avatar{ID: testAvatarID, UserID: "user-1"}

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)

		found, err := d.service.Metadata(context.Background(), testAvatarID)
		require.NoError(t, err)
		assert.Equal(t, stored, found)
	})

	t.Run("Metadata not found", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testMissingID).Return(nil, repository.ErrNotFound)

		_, err := d.service.Metadata(context.Background(), testMissingID)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Metadata with a malformed id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		_, err := d.service.Metadata(context.Background(), "not-a-uuid")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()

		avatars := []model.Avatar{{ID: "avatar-2"}, {ID: testAvatarID}}

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetUserAvatars(gomock.Any(), "user-1").Return(avatars, nil)

		list, err := d.service.ListUserAvatars(context.Background(), "user-1")
		require.NoError(t, err)
		assert.Equal(t, avatars, list)
	})

	t.Run("List failure", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetUserAvatars(gomock.Any(), "user-1").Return(nil, errors.New("db is down"))

		_, err := d.service.ListUserAvatars(context.Background(), "user-1")
		require.Error(t, err)
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	stored := &model.Avatar{
		ID:              testAvatarID,
		UserID:          "user-1",
		S3Key:           "avatars/avatar-1/original.png",
		ThumbnailS3Keys: map[string]string{model.ThumbnailSmall: "thumbnails/avatar-1/100x100.jpg"},
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)
		d.storage.EXPECT().SoftDeleteAvatar(gomock.Any(), testAvatarID).Return(nil)
		d.producer.EXPECT().Publish(gomock.Any(), testAvatarID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, event model.Event) error {
				assert.Equal(t, model.EventAvatarDeleted, event.Type)
				assert.Contains(t, string(event.Payload), "thumbnails/avatar-1/100x100.jpg")
				return nil
			})

		require.NoError(t, d.service.Delete(context.Background(), testAvatarID, "user-1"))
	})

	t.Run("Foreign avatar", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)

		err := d.service.Delete(context.Background(), testAvatarID, "user-2")
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("Without user id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		err := d.service.Delete(context.Background(), testAvatarID, "")
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("Not found", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testMissingID).Return(nil, repository.ErrNotFound)

		err := d.service.Delete(context.Background(), testMissingID, "user-1")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("Malformed id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		err := d.service.Delete(context.Background(), "not-a-uuid", "user-1")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestDeleteUserAvatar(t *testing.T) {
	t.Parallel()

	stored := &model.Avatar{ID: testAvatarID, UserID: "user-1", S3Key: "avatars/avatar-1/original.png"}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().GetLastUserAvatar(gomock.Any(), "user-1").Return(stored, nil)
		d.storage.EXPECT().GetAvatar(gomock.Any(), testAvatarID).Return(stored, nil)
		d.storage.EXPECT().SoftDeleteAvatar(gomock.Any(), testAvatarID).Return(nil)
		d.producer.EXPECT().Publish(gomock.Any(), testAvatarID, gomock.Any()).Return(nil)

		require.NoError(t, d.service.DeleteUserAvatar(context.Background(), "user-1", "user-1"))
	})

	t.Run("Foreign user", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		err := d.service.DeleteUserAvatar(context.Background(), "user-1", "user-2")
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("Without user id", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)

		err := d.service.DeleteUserAvatar(context.Background(), "user-1", "")
		assert.ErrorIs(t, err, ErrUnauthorized)
	})
}

func TestCheck(t *testing.T) {
	t.Parallel()

	t.Run("All components are up", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().Ping(gomock.Any()).Return(nil)
		d.objects.EXPECT().Ping(gomock.Any()).Return(nil)
		d.producer.EXPECT().Ping(gomock.Any()).Return(nil)

		health := d.service.Check(context.Background())
		assert.Equal(t, StatusUp, health.Status)
		assert.Equal(t, StatusUp, health.Components["database"])
	})

	t.Run("Broker is down", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t, maxUploadSize)
		d.storage.EXPECT().Ping(gomock.Any()).Return(nil)
		d.objects.EXPECT().Ping(gomock.Any()).Return(nil)
		d.producer.EXPECT().Ping(gomock.Any()).Return(errors.New("no brokers"))

		health := d.service.Check(context.Background())
		assert.Equal(t, StatusDown, health.Status)
		assert.Equal(t, StatusDown, health.Components["broker"])
		assert.Equal(t, StatusUp, health.Components["s3"])
	})
}

func TestServiceError(t *testing.T) {
	t.Parallel()

	assert.Contains(t, NotFoundError().Error(), "Avatar not found")
	assert.Contains(t, ForbiddenError().Error(), "own avatars")
	assert.Contains(t, UnsupportedFormatError().Error(), "jpeg, png, webp")
}

func TestThumbnailKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "thumbnails/avatar-1/100x100.jpg", ThumbnailKey("avatar-1", model.ThumbnailSmall))
}
