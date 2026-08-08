// Package middleware содержит промежуточные обработчики HTTP-запросов.
package middleware

import (
	"context"
	"net/http"
	"regexp"

	"github.com/vrnvgasu/gophprofile/internal/handler/response"
)

const UserIDHeader = "X-User-ID"

const maxUserIDLen = 255

var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9._@-]+$`)

type contextKey struct{}

var userIDKey = contextKey{}

func UserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get(UserIDHeader)

		if userID != "" && !IsValidUserID(userID) {
			response.JSON(w, http.StatusBadRequest, response.Error{
				Error:   "Invalid user id",
				Details: "X-User-ID contains unsupported characters",
			})

			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromCtx возвращает идентификатор пользователя из контекста запроса.
// Для запросов без заголовка возвращает пустую строку.
func UserIDFromCtx(ctx context.Context) string {
	userID, _ := ctx.Value(userIDKey).(string)
	return userID
}

// IsValidUserID проверяет длину и набор символов идентификатора.
// Нужна там, где идентификатор приходит не заголовком, а полем формы.
func IsValidUserID(userID string) bool {
	return len(userID) <= maxUserIDLen && userIDPattern.MatchString(userID)
}
