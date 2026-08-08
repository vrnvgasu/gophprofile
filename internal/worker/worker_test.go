package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/repository"
	repomocks "github.com/vrnvgasu/gophprofile/internal/repository/mocks"
	"github.com/vrnvgasu/gophprofile/internal/storage/s3"
	"github.com/vrnvgasu/gophprofile/internal/worker/mocks"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 50, A: 255})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

type deps struct {
	storage *repomocks.MockAvatarStorage
	objects *mocks.MockObjectStorage
	worker  *Worker
}

func newDeps(t *testing.T) *deps {
	t.Helper()

	ctrl := gomock.NewController(t)
	storage := repomocks.NewMockAvatarStorage(ctrl)
	objects := mocks.NewMockObjectStorage(ctrl)

	return &deps{storage: storage, objects: objects, worker: New(storage, objects)}
}

func uploadEvent(t *testing.T, payload model.AvatarUploadEvent) model.Event {
	t.Helper()

	event, err := model.NewEvent("message-1", model.EventAvatarUploaded, payload)
	require.NoError(t, err)

	return event
}

func TestHandleUploadEvent(t *testing.T) {
	t.Parallel()

	event := model.AvatarUploadEvent{
		AvatarID: "avatar-1",
		UserID:   "user-1",
		S3Key:    "avatars/avatar-1/original.png",
	}

	t.Run("Creates both thumbnails", func(t *testing.T) {
		t.Parallel()

		original := testPNG(t, 400, 200)

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").
			Return(&model.Avatar{ID: "avatar-1", ProcessingStatus: model.ProcessingStatusPending}, nil)
		d.storage.EXPECT().
			SetProcessingStatus(gomock.Any(), "avatar-1", model.ProcessingStatusProcessing).
			Return(nil)
		d.objects.EXPECT().Get(gomock.Any(), event.S3Key).
			Return(io.NopCloser(bytes.NewReader(original)), nil)

		uploaded := map[string][]byte{}
		d.objects.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/jpeg").
			DoAndReturn(func(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
				data, err := io.ReadAll(r)
				require.NoError(t, err)
				uploaded[key] = data

				return nil
			}).Times(2)
		d.storage.EXPECT().SaveThumbnails(gomock.Any(), "avatar-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, thumbnails map[string]string) error {
				assert.Equal(t, "thumbnails/avatar-1/100x100.jpg", thumbnails[model.ThumbnailSmall])
				assert.Equal(t, "thumbnails/avatar-1/300x300.jpg", thumbnails[model.ThumbnailMedium])

				return nil
			})

		require.NoError(t, d.worker.Handle(context.Background(), uploadEvent(t, event)))
		require.Len(t, uploaded, 2)

		// Проверяем, что миниатюры действительно нужного размера.
		for key, expected := range map[string]int{
			"thumbnails/avatar-1/100x100.jpg": 100,
			"thumbnails/avatar-1/300x300.jpg": 300,
		} {
			cfg, format, err := image.DecodeConfig(bytes.NewReader(uploaded[key]))
			require.NoError(t, err)
			assert.Equal(t, "jpeg", format)
			assert.Equal(t, expected, cfg.Width)
			assert.Equal(t, expected, cfg.Height)
		}
	})

	t.Run("Already processed event is skipped", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").
			Return(&model.Avatar{ID: "avatar-1", ProcessingStatus: model.ProcessingStatusCompleted}, nil)

		require.NoError(t, d.worker.HandleUploadEvent(context.Background(), event))
	})

	t.Run("Deleted avatar is skipped", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").Return(nil, repository.ErrNotFound)

		require.NoError(t, d.worker.HandleUploadEvent(context.Background(), event))
	})

	t.Run("Missing object marks processing as failed", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").
			Return(&model.Avatar{ID: "avatar-1", ProcessingStatus: model.ProcessingStatusPending}, nil)
		d.storage.EXPECT().
			SetProcessingStatus(gomock.Any(), "avatar-1", model.ProcessingStatusProcessing).
			Return(nil)
		d.objects.EXPECT().Get(gomock.Any(), event.S3Key).Return(nil, s3.ErrNotFound)
		d.storage.EXPECT().
			SetProcessingStatus(gomock.Any(), "avatar-1", model.ProcessingStatusFailed).
			Return(nil)

		err := d.worker.HandleUploadEvent(context.Background(), event)
		require.ErrorIs(t, err, s3.ErrNotFound)
	})

	t.Run("Broken image marks processing as failed", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").
			Return(&model.Avatar{ID: "avatar-1"}, nil)
		d.storage.EXPECT().
			SetProcessingStatus(gomock.Any(), "avatar-1", model.ProcessingStatusProcessing).
			Return(nil)
		d.objects.EXPECT().Get(gomock.Any(), event.S3Key).
			Return(io.NopCloser(bytes.NewReader([]byte("not an image"))), nil)
		d.storage.EXPECT().
			SetProcessingStatus(gomock.Any(), "avatar-1", model.ProcessingStatusFailed).
			Return(nil)

		require.Error(t, d.worker.HandleUploadEvent(context.Background(), event))
	})

	t.Run("Storage failure is reported", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.storage.EXPECT().GetAvatar(gomock.Any(), "avatar-1").Return(nil, errors.New("db is down"))

		require.Error(t, d.worker.HandleUploadEvent(context.Background(), event))
	})
}

func TestHandleDeleteEvent(t *testing.T) {
	t.Parallel()

	t.Run("Removes every key", func(t *testing.T) {
		t.Parallel()

		keys := []string{"avatars/avatar-1/original.png", "thumbnails/avatar-1/100x100.jpg"}

		d := newDeps(t)
		for _, key := range keys {
			d.objects.EXPECT().Delete(gomock.Any(), key).Return(nil)
		}

		event, err := model.NewEvent("message-2", model.EventAvatarDeleted, model.AvatarDeleteEvent{
			AvatarID: "avatar-1",
			S3Keys:   keys,
		})
		require.NoError(t, err)

		require.NoError(t, d.worker.Handle(context.Background(), event))
	})

	t.Run("Repeated delete is not an error", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.objects.EXPECT().Delete(gomock.Any(), "avatars/avatar-1/original.png").Return(s3.ErrNotFound)

		err := d.worker.HandleDeleteEvent(context.Background(), model.AvatarDeleteEvent{
			AvatarID: "avatar-1",
			S3Keys:   []string{"avatars/avatar-1/original.png"},
		})
		require.NoError(t, err)
	})
}

func TestHandleUnknownEvent(t *testing.T) {
	t.Parallel()

	d := newDeps(t)

	err := d.worker.Handle(context.Background(), model.Event{
		ID:      "message-3",
		Type:    "avatar.unknown",
		Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
}

func TestHandleBrokenPayload(t *testing.T) {
	t.Parallel()

	d := newDeps(t)

	err := d.worker.Handle(context.Background(), model.Event{
		ID:      "message-4",
		Type:    model.EventAvatarUploaded,
		Payload: json.RawMessage(`{"avatar_id":`),
	})
	require.Error(t, err)
}
