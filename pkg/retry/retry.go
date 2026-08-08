// Package retry - утилиты для повторного выполнения операций при ошибках.
package retry

import (
	"errors"
	"time"
)

type Config struct {
	MaxRetries         int
	StartRetryInterval time.Duration
	Multiplier         int
}

func DefaultConfig() *Config {
	return &Config{
		MaxRetries:         3,
		StartRetryInterval: 1 * time.Second,
		Multiplier:         2,
	}
}

type RetryableError struct {
	Err error
}

func NewRetryableError(err error) error {
	return &RetryableError{Err: err}
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

func RetryWithSettings(f func() error, cnf *Config) error {
	if cnf == nil {
		cnf = DefaultConfig()
	}

	retryInterval := cnf.StartRetryInterval
	attempt := 0

	for {
		err := f()
		if err == nil {
			return nil
		}

		var re *RetryableError
		if !errors.As(err, &re) {
			return err
		}
		if attempt >= cnf.MaxRetries {
			return err
		}

		time.Sleep(retryInterval)
		attempt++
		retryInterval *= time.Duration(cnf.Multiplier)
	}
}

func Retry(f func() error) error {
	return RetryWithSettings(f, DefaultConfig())
}
