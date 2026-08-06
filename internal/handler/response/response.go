// Package response содержит помощники для формирования HTTP-ответов.
package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vrnvgasu/gophprofile/internal/logger"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

type Error struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
	MaxSize int64  `json:"max_size,omitempty"`
}

func JSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)

	if payload == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Log.Errorw("response.JSON Encode", "error", err)
	}
}

func ResponseError(w http.ResponseWriter, r *http.Request, err error) {
	var serviceErr *avatar.ServiceError
	if errors.As(err, &serviceErr) {
		JSON(w, serviceErr.HTTPCode, Error{
			Error:   serviceErr.Message,
			Details: serviceErr.Details,
			MaxSize: serviceErr.MaxSize,
		})

		return
	}

	LogError(r, err)

	JSON(w, http.StatusInternalServerError, Error{Error: "Internal server error"})
}

func LogError(r *http.Request, err error) {
	logger.Log.Errorw("http request",
		"uri", r.RequestURI,
		"method", r.Method,
		"error", err,
	)
}
