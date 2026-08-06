package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		header         string
		expectedStatus int
		expectedUserID string
	}{
		{
			name:           "Valid identifier",
			header:         "user-1",
			expectedStatus: http.StatusOK,
			expectedUserID: "user-1",
		},
		{
			name:           "Email as identifier",
			header:         "user@example.com",
			expectedStatus: http.StatusOK,
			expectedUserID: "user@example.com",
		},
		{
			name:           "No header",
			header:         "",
			expectedStatus: http.StatusOK,
			expectedUserID: "",
		},
		{
			name:           "Path traversal attempt",
			header:         "../../etc/passwd",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Space inside",
			header:         "user 1",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Too long",
			header:         strings.Repeat("a", 256),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var captured string
			handler := UserID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				captured = UserIDFromCtx(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars", nil)
			if tt.header != "" {
				req.Header.Set(UserIDHeader, tt.header)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.expectedUserID, captured)
			}
		})
	}
}

func TestUserIDFromCtxWithoutMiddleware(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Empty(t, UserIDFromCtx(req.Context()))
}

func TestLogger(t *testing.T) {
	t.Parallel()

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, err := w.Write([]byte("done"))
		require.NoError(t, err)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "done", rec.Body.String())
}
