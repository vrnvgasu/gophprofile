package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vrnvgasu/gophprofile/internal/handler/mocks"
)

func newStaticServer(t *testing.T, staticDir string) *httptest.Server {
	t.Helper()

	app := mocks.NewMockApp(gomock.NewController(t))
	srv := httptest.NewServer(NewRouter(NewHandler(app, testMaxUploadSize), staticDir))
	t.Cleanup(srv.Close)

	return srv
}

func TestStaticRoutes(t *testing.T) {
	t.Parallel()

	staticDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>spa</html>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "app.js"), []byte("console.log(1)"), 0o600))

	srv := newStaticServer(t, staticDir)

	t.Run("Serves the index page", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, http.MethodGet, srv.URL+"/", nil, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Serves an existing file", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, http.MethodGet, srv.URL+"/app.js", nil, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Unknown path falls back to the index page", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, http.MethodGet, srv.URL+"/gallery/user-1", nil, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestRouterWithoutStatic(t *testing.T) {
	t.Parallel()

	srv := newStaticServer(t, "")

	resp := doRequest(t, http.MethodGet, srv.URL+"/", nil, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRouterWithMissingStaticDir(t *testing.T) {
	t.Parallel()

	srv := newStaticServer(t, filepath.Join(t.TempDir(), "does-not-exist"))

	resp := doRequest(t, http.MethodGet, srv.URL+"/", nil, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
