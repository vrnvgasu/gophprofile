package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastConfig — конфигурация с короткими паузами, чтобы тесты не ждали.
func fastConfig(maxRetries int) *Config {
	return &Config{
		MaxRetries:         maxRetries,
		StartRetryInterval: time.Millisecond,
		Multiplier:         2,
	}
}

func TestRetryWithSettings(t *testing.T) {
	t.Parallel()

	t.Run("Succeeds on the first attempt", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := RetryWithSettings(func() error {
			attempts++
			return nil
		}, fastConfig(3))

		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("Succeeds after retries", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := RetryWithSettings(func() error {
			attempts++
			if attempts < 3 {
				return NewRetryableError(errors.New("connection refused"))
			}

			return nil
		}, fastConfig(3))

		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("Permanent error is not retried", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := RetryWithSettings(func() error {
			attempts++
			return errors.New("constraint violation")
		}, fastConfig(3))

		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("Gives up after the limit", func(t *testing.T) {
		t.Parallel()

		attempts := 0
		err := RetryWithSettings(func() error {
			attempts++
			return NewRetryableError(errors.New("connection refused"))
		}, fastConfig(2))

		require.Error(t, err)
		assert.Equal(t, 3, attempts, "первая попытка плюс две повторные")
	})

	t.Run("Nil config falls back to defaults", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, RetryWithSettings(func() error { return nil }, nil))
	})
}

func TestRetryableError(t *testing.T) {
	t.Parallel()

	origin := errors.New("origin")
	wrapped := NewRetryableError(origin)

	assert.Equal(t, "origin", wrapped.Error())
	assert.ErrorIs(t, wrapped, origin)
}

func TestRetry(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := Retry(func() error {
		attempts++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, time.Second, cfg.StartRetryInterval)
	assert.Equal(t, 2, cfg.Multiplier)
}
