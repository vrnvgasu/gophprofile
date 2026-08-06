package avatar

import (
	"fmt"
	"net/http"
)

type ServiceError struct {
	HTTPCode int
	Message  string
	Details  string
	MaxSize  int64
}

func (e *ServiceError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("[%d]: %s", e.HTTPCode, e.Message)
	}

	return fmt.Sprintf("[%d]: %s: %s", e.HTTPCode, e.Message, e.Details)
}

func BadRequestError(message, details string) error {
	return &ServiceError{
		HTTPCode: http.StatusBadRequest,
		Message:  message,
		Details:  details,
	}
}

func UnauthorizedError() error {
	return &ServiceError{
		HTTPCode: http.StatusUnauthorized,
		Message:  "Unauthorized",
		Details:  "X-User-ID header is required",
	}
}

func ForbiddenError() error {
	return &ServiceError{
		HTTPCode: http.StatusForbidden,
		Message:  "Forbidden",
		Details:  "You can only delete your own avatars",
	}
}

func NotFoundError() error {
	return &ServiceError{
		HTTPCode: http.StatusNotFound,
		Message:  "Avatar not found",
	}
}

func TooLargeError(maxSize int64) error {
	return &ServiceError{
		HTTPCode: http.StatusRequestEntityTooLarge,
		Message:  "File too large",
		MaxSize:  maxSize,
	}
}

func UnsupportedFormatError() error {
	return &ServiceError{
		HTTPCode: http.StatusBadRequest,
		Message:  "Invalid file format",
		Details:  "Supported formats: jpeg, png, webp",
	}
}
