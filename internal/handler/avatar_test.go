package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vrnvgasu/gophprofile/internal/handler/mocks"
	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

const testMaxUploadSize = 1 << 20

func newTestServer(t *testing.T) (*mocks.MockApp, *httptest.Server) {
	t.Helper()

	ctrl := gomock.NewController(t)
	app := mocks.NewMockApp(ctrl)
	srv := httptest.NewServer(NewRouter(NewHandler(app, testMaxUploadSize), "", nil))
	t.Cleanup(srv.Close)

	return app, srv
}

func multipartBody(t *testing.T, fieldName, fileName string, content []byte) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return &buf, writer.FormDataContentType()
}

func doRequest(t *testing.T, method, url string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	require.NoError(t, err)

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func TestUploadAvatar(t *testing.T) {
	t.Parallel()

	created := &model.Avatar{
		ID:               "avatar-1",
		UserID:           "user-1",
		ProcessingStatus: model.ProcessingStatusPending,
		CreatedAt:        time.Now().UTC(),
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), "user-1", "photo.png", []byte("image-bytes")).Return(created, nil)

		body, contentType := multipartBody(t, "file", "photo.png", []byte("image-bytes"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var payload uploadResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		assert.Equal(t, "avatar-1", payload.ID)
		assert.Equal(t, "/api/v1/avatars/avatar-1", payload.URL)
		assert.Equal(t, "processing", payload.Status)
	})

	t.Run("Missing file field", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		body, contentType := multipartBody(t, "attachment", "photo.png", []byte("image-bytes"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Field name from the bundled frontend", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), "user-1", "photo.png", []byte("image-bytes")).Return(created, nil)

		body, contentType := multipartBody(t, "image", "photo.png", []byte("image-bytes"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("Body exceeds the limit", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		body, contentType := multipartBody(t, "file", "photo.png", bytes.Repeat([]byte("a"), 3<<20))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)

		var payload struct {
			Error   string `json:"error"`
			MaxSize int64  `json:"max_size"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		assert.Equal(t, int64(testMaxUploadSize), payload.MaxSize)
	})

	t.Run("Service rejects the format", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, avatar.UnsupportedFormatError())

		body, contentType := multipartBody(t, "file", "notes.txt", []byte("plain text"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var payload struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		assert.Equal(t, "Invalid file format", payload.Error)
	})

	t.Run("Invalid user id", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		body, contentType := multipartBody(t, "file", "photo.png", []byte("image-bytes"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user 1/../etc",
		})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Unexpected service failure", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("db is down"))

		body, contentType := multipartBody(t, "file", "photo.png", []byte("image-bytes"))
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/v1/avatars", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestGetAvatar(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Download(gomock.Any(), "avatar-1", "").Return(&avatar.Content{
			Body:     io.NopCloser(strings.NewReader("image-bytes")),
			MimeType: "image/png",
			ETag:     "avatar-1-original",
		}, nil)

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/avatar-1", nil, nil)

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
		assert.Equal(t, `"avatar-1-original"`, resp.Header.Get("ETag"))
		assert.Equal(t, "max-age=86400", resp.Header.Get("Cache-Control"))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "image-bytes", string(body))
	})

	t.Run("Thumbnail size", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Download(gomock.Any(), "avatar-1", "100x100").Return(&avatar.Content{
			Body:     io.NopCloser(strings.NewReader("thumb")),
			MimeType: "image/jpeg",
			ETag:     "avatar-1-100x100",
		}, nil)

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/avatar-1?size=100x100", nil, nil)

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	})

	t.Run("Unknown size", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/avatar-1?size=42x42", nil, nil)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Cached by the client", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Download(gomock.Any(), "avatar-1", "").Return(&avatar.Content{
			Body:     io.NopCloser(strings.NewReader("image-bytes")),
			MimeType: "image/png",
			ETag:     "avatar-1-original",
		}, nil)

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/avatar-1", nil, map[string]string{
			"If-None-Match": `"avatar-1-original"`,
		})

		assert.Equal(t, http.StatusNotModified, resp.StatusCode)
	})

	t.Run("Not found", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Download(gomock.Any(), "missing", "").Return(nil, avatar.NotFoundError())

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/missing", nil, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestGetUserAvatar(t *testing.T) {
	t.Parallel()

	app, srv := newTestServer(t)
	app.EXPECT().DownloadUserAvatar(gomock.Any(), "user-1", "300x300").Return(&avatar.Content{
		Body:     io.NopCloser(strings.NewReader("thumb")),
		MimeType: "image/jpeg",
		ETag:     "avatar-1-300x300",
	}, nil)

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/users/user-1/avatar?size=300x300", nil, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetAvatarMetadata(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		stored := &model.Avatar{
			ID:               "avatar-1",
			UserID:           "user-1",
			FileName:         "photo.png",
			MimeType:         "image/png",
			SizeBytes:        1024,
			Width:            800,
			Height:           600,
			ProcessingStatus: model.ProcessingStatusCompleted,
			ThumbnailS3Keys: map[string]string{
				model.ThumbnailSmall:  "thumbnails/avatar-1/100x100.jpg",
				model.ThumbnailMedium: "thumbnails/avatar-1/300x300.jpg",
			},
		}

		app, srv := newTestServer(t)
		app.EXPECT().Metadata(gomock.Any(), "avatar-1").Return(stored, nil)

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/avatar-1/metadata", nil, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var payload metadataResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		assert.Equal(t, "photo.png", payload.FileName)
		assert.Equal(t, int64(1024), payload.Size)
		assert.Equal(t, 800, payload.Dimensions.Width)
		assert.Equal(t, "completed", payload.Status)
		require.Len(t, payload.Thumbnails, 2)
		assert.Equal(t, "100x100", payload.Thumbnails[0].Size)
		assert.Equal(t, "/api/v1/avatars/avatar-1?size=100x100", payload.Thumbnails[0].URL)
	})

	t.Run("Not found", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Metadata(gomock.Any(), "missing").Return(nil, avatar.NotFoundError())

		resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/avatars/missing/metadata", nil, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestListUserAvatars(t *testing.T) {
	t.Parallel()

	app, srv := newTestServer(t)
	app.EXPECT().ListUserAvatars(gomock.Any(), "user-1").
		Return([]model.Avatar{{ID: "avatar-2"}, {ID: "avatar-1"}}, nil)

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/users/user-1/avatars", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload []metadataResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload, 2)
	assert.Equal(t, "avatar-2", payload[0].ID)
}

func TestDeleteAvatar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serviceErr     error
		userID         string
		expectedStatus int
	}{
		{name: "Success", userID: "user-1", expectedStatus: http.StatusNoContent},
		{
			name:           "Foreign avatar",
			serviceErr:     avatar.ForbiddenError(),
			userID:         "user-2",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Not found",
			serviceErr:     avatar.NotFoundError(),
			userID:         "user-1",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Without user id",
			serviceErr:     avatar.UnauthorizedError(),
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, srv := newTestServer(t)
			app.EXPECT().Delete(gomock.Any(), "avatar-1", tt.userID).Return(tt.serviceErr)

			headers := map[string]string{}
			if tt.userID != "" {
				headers["X-User-ID"] = tt.userID
			}

			resp := doRequest(t, http.MethodDelete, srv.URL+"/api/v1/avatars/avatar-1", nil, headers)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestDeleteUserAvatar(t *testing.T) {
	t.Parallel()

	app, srv := newTestServer(t)
	app.EXPECT().DeleteUserAvatar(gomock.Any(), "user-1", "user-1").Return(nil)

	resp := doRequest(t, http.MethodDelete, srv.URL+"/api/v1/users/user-1/avatar", nil, map[string]string{
		"X-User-ID": "user-1",
	})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestHealth(t *testing.T) {
	t.Parallel()

	t.Run("Everything is up", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Check(gomock.Any()).Return(avatar.Health{
			Status:     avatar.StatusUp,
			Components: map[string]string{"database": avatar.StatusUp},
		})

		resp := doRequest(t, http.MethodGet, srv.URL+"/health", nil, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var payload avatar.Health
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		assert.Equal(t, avatar.StatusUp, payload.Status)
	})

	t.Run("Dependency is down", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Check(gomock.Any()).Return(avatar.Health{
			Status:     avatar.StatusDown,
			Components: map[string]string{"database": avatar.StatusDown},
		})

		resp := doRequest(t, http.MethodGet, srv.URL+"/health", nil, nil)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})
}
