package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vrnvgasu/gophprofile/internal/repository"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	classifier := newPostgresErrorClassifier()

	tests := []struct {
		name     string
		err      error
		expected retriability
	}{
		{
			name:     "Network error before the server replied",
			err:      errors.New("connection refused"),
			expected: retriable,
		},
		{
			name:     "Connection exception",
			err:      &pgconn.PgError{Code: pgerrcode.ConnectionException},
			expected: retriable,
		},
		{
			name:     "Unique violation",
			err:      &pgconn.PgError{Code: pgerrcode.UniqueViolation},
			expected: nonRetriable,
		},
		{
			name:     "Record not found",
			err:      fmt.Errorf("wrapped: %w", repository.ErrNotFound),
			expected: nonRetriable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, classifier.classify(tt.err))
		})
	}
}

func TestWithRetry(t *testing.T) {
	t.Parallel()

	t.Run("Permanent error stops immediately", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := withRetry(func() error {
			attempts++
			return &pgconn.PgError{Code: pgerrcode.UniqueViolation}
		}, newPostgresErrorClassifier())

		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("Success without retries", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := withRetry(func() error {
			attempts++
			return nil
		}, newPostgresErrorClassifier())

		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})
}
