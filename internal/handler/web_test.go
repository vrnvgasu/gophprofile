package handler

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

func webFormBody(t *testing.T, fileName string, content []byte, fields map[string]string) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}

	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return &buf, writer.FormDataContentType()
}

func doRequestNoRedirect(t *testing.T, method, url string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	require.NoError(t, err)

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	resp, err := client.Do(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func TestWebUpload(t *testing.T) {
	t.Parallel()

	created := &model.Avatar{
		ID:               "avatar-1",
		UserID:           "user-1",
		ProcessingStatus: model.ProcessingStatusPending,
		CreatedAt:        time.Now().UTC(),
	}

	t.Run("Redirects to the gallery after upload", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), "user-1", "photo.png", []byte("image-bytes")).Return(created, nil)

		body, contentType := webFormBody(t, "photo.png", []byte("image-bytes"),
			map[string]string{"userId": "user-1"})
		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", body,
			map[string]string{"Content-Type": contentType})

		require.Equal(t, http.StatusSeeOther, resp.StatusCode)
		assert.Equal(t, "/web/gallery/user-1", resp.Header.Get("Location"))
	})

	t.Run("Takes the user id from the header", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), "user-1", "photo.png", []byte("image-bytes")).Return(created, nil)

		body, contentType := webFormBody(t, "photo.png", []byte("image-bytes"), nil)
		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", body, map[string]string{
			"Content-Type": contentType,
			"X-User-ID":    "user-1",
		})

		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	})

	t.Run("Without a user id", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		body, contentType := webFormBody(t, "photo.png", []byte("image-bytes"), nil)
		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", body,
			map[string]string{"Content-Type": contentType})

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("With an invalid user id in the form", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		body, contentType := webFormBody(t, "photo.png", []byte("image-bytes"),
			map[string]string{"userId": "../../etc"})
		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", body,
			map[string]string{"Content-Type": contentType})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Without a file", func(t *testing.T) {
		t.Parallel()

		_, srv := newTestServer(t)

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		require.NoError(t, writer.WriteField("userId", "user-1"))
		require.NoError(t, writer.Close())

		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", &buf,
			map[string]string{"Content-Type": writer.FormDataContentType()})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Upload failure", func(t *testing.T) {
		t.Parallel()

		app, srv := newTestServer(t)
		app.EXPECT().Upload(gomock.Any(), "user-1", "photo.png", gomock.Any()).
			Return(nil, avatar.UnsupportedFormatError())

		body, contentType := webFormBody(t, "photo.png", []byte("not-an-image"),
			map[string]string{"userId": "user-1"})
		resp := doRequestNoRedirect(t, http.MethodPost, srv.URL+"/web/upload", body,
			map[string]string{"Content-Type": contentType})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestWebPages(t *testing.T) {
	t.Parallel()

	staticDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>spa</html>"), 0o600))

	srv := newStaticServer(t, staticDir)

	t.Run("Upload form", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, http.MethodGet, srv.URL+"/web/upload", nil, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		page, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(page), "spa")
	})

	t.Run("User gallery", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, http.MethodGet, srv.URL+"/web/gallery/user-1", nil, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestWebPagesWithoutStatic(t *testing.T) {
	t.Parallel()

	srv := newStaticServer(t, "")

	for _, path := range []string{"/web/upload", "/web/gallery/user-1"} {
		resp := doRequest(t, http.MethodGet, srv.URL+path, nil, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestServeIndexWithMissingFile(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/upload", nil)

	serveIndex(t.TempDir())(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
