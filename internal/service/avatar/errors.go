package avatar

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrNotFound          = errors.New("not found")
	ErrTooLarge          = errors.New("too large")
	ErrUnsupportedFormat = errors.New("unsupported format")
)

type ServiceError struct {
	Kind    error
	Message string
	Details string
	MaxSize int64
}

func (e *ServiceError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("[%s]: %s", e.Kind, e.Message)
	}

	return fmt.Sprintf("[%s]: %s: %s", e.Kind, e.Message, e.Details)
}

func (e *ServiceError) Unwrap() error {
	return e.Kind
}

func BadRequestError(message, details string) error {
	return &ServiceError{
		Kind:    ErrInvalidInput,
		Message: message,
		Details: details,
	}
}

func UnauthorizedError() error {
	return &ServiceError{
		Kind:    ErrUnauthorized,
		Message: "Unauthorized",
		Details: "X-User-ID header is required",
	}
}

func ForbiddenError() error {
	return &ServiceError{
		Kind:    ErrForbidden,
		Message: "Forbidden",
		Details: "You can only delete your own avatars",
	}
}

func NotFoundError() error {
	return &ServiceError{
		Kind:    ErrNotFound,
		Message: "Avatar not found",
	}
}

func TooLargeError(maxSize int64) error {
	return &ServiceError{
		Kind:    ErrTooLarge,
		Message: "File too large",
		MaxSize: maxSize,
	}
}

func UnsupportedFormatError() error {
	return &ServiceError{
		Kind:    ErrUnsupportedFormat,
		Message: "Invalid file format",
		Details: "Supported formats: jpeg, png, webp",
	}
}
