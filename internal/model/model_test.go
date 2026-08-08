package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThumbnailKey(t *testing.T) {
	t.Parallel()

	avatar := &Avatar{
		ThumbnailS3Keys: map[string]string{ThumbnailSmall: "thumbnails/avatar-1/100x100.jpg"},
	}

	key, ok := avatar.ThumbnailKey(ThumbnailSmall)
	assert.True(t, ok)
	assert.Equal(t, "thumbnails/avatar-1/100x100.jpg", key)

	_, ok = avatar.ThumbnailKey(ThumbnailMedium)
	assert.False(t, ok)
}

func TestAllS3Keys(t *testing.T) {
	t.Parallel()

	t.Run("With thumbnails", func(t *testing.T) {
		t.Parallel()

		avatar := &Avatar{
			S3Key: "avatars/avatar-1/original.png",
			ThumbnailS3Keys: map[string]string{
				ThumbnailMedium: "thumbnails/avatar-1/300x300.jpg",
				ThumbnailSmall:  "thumbnails/avatar-1/100x100.jpg",
			},
		}

		// Порядок ключей детерминирован: оригинал, затем миниатюры по возрастанию размера.
		assert.Equal(t, []string{
			"avatars/avatar-1/original.png",
			"thumbnails/avatar-1/100x100.jpg",
			"thumbnails/avatar-1/300x300.jpg",
		}, avatar.AllS3Keys())
	})

	t.Run("Without thumbnails", func(t *testing.T) {
		t.Parallel()

		avatar := &Avatar{S3Key: "avatars/avatar-1/original.png"}
		assert.Equal(t, []string{"avatars/avatar-1/original.png"}, avatar.AllS3Keys())
	})
}

func TestNewEvent(t *testing.T) {
	t.Parallel()

	event, err := NewEvent("message-1", EventAvatarUploaded, AvatarUploadEvent{
		AvatarID: "avatar-1",
		UserID:   "user-1",
		S3Key:    "avatars/avatar-1/original.png",
	})
	require.NoError(t, err)

	assert.Equal(t, "message-1", event.ID)
	assert.Equal(t, EventAvatarUploaded, event.Type)

	var payload AvatarUploadEvent
	require.NoError(t, json.Unmarshal(event.Payload, &payload))
	assert.Equal(t, "avatar-1", payload.AvatarID)
}

func TestNewEventWithBrokenPayload(t *testing.T) {
	t.Parallel()

	_, err := NewEvent("message-1", EventAvatarUploaded, make(chan int))
	require.Error(t, err)
}
