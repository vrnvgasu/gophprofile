package handler

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vrnvgasu/gophprofile/internal/handler/middleware"
	"github.com/vrnvgasu/gophprofile/internal/handler/response"
	"github.com/vrnvgasu/gophprofile/internal/model"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

const multipartOverhead = 1 << 20

// cacheMaxAge — время жизни изображения в кеше клиента, в секундах.
const cacheMaxAge = 86400

// UploadAvatar обрабатывает POST /api/v1/avatars.
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())

	data, fileName, err := h.readUploadedFile(w, r)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	created, err := h.app.Upload(r.Context(), userID, fileName, data)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	response.JSON(w, http.StatusCreated, newUploadResponse(created))
}

func (h *Handler) readUploadedFile(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	// Ограничиваем тело запроса до того, как оно будет прочитано в память.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadSize+multipartOverhead)

	file, header, err := formFile(r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, "", avatar.TooLargeError(h.maxUploadSize)
		}

		return nil, "", avatar.BadRequestError("Invalid request", "form field \"file\" is required")
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, "", avatar.TooLargeError(h.maxUploadSize)
		}

		return nil, "", err
	}

	return data, header.Filename, nil
}

// GetAvatar обрабатывает GET /api/v1/avatars/{avatar_id}.
func (h *Handler) GetAvatar(w http.ResponseWriter, r *http.Request) {
	size, err := parseSize(r)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	content, err := h.app.Download(r.Context(), chi.URLParam(r, "avatar_id"), size)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	writeContent(w, r, content)
}

// GetUserAvatar обрабатывает GET /api/v1/users/{user_id}/avatar.
func (h *Handler) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	size, err := parseSize(r)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	content, err := h.app.DownloadUserAvatar(r.Context(), chi.URLParam(r, "user_id"), size)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	writeContent(w, r, content)
}

// GetAvatarMetadata обрабатывает GET /api/v1/avatars/{avatar_id}/metadata.
func (h *Handler) GetAvatarMetadata(w http.ResponseWriter, r *http.Request) {
	found, err := h.app.Metadata(r.Context(), chi.URLParam(r, "avatar_id"))
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, newMetadataResponse(found))
}

// ListUserAvatars обрабатывает GET /api/v1/users/{user_id}/avatars.
func (h *Handler) ListUserAvatars(w http.ResponseWriter, r *http.Request) {
	avatars, err := h.app.ListUserAvatars(r.Context(), chi.URLParam(r, "user_id"))
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	response.JSON(w, http.StatusOK, newMetadataList(avatars))
}

// DeleteAvatar обрабатывает DELETE /api/v1/avatars/{avatar_id}.
func (h *Handler) DeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())

	if err := h.app.Delete(r.Context(), chi.URLParam(r, "avatar_id"), userID); err != nil {
		response.ResponseError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteUserAvatar обрабатывает DELETE /api/v1/users/{user_id}/avatar.
func (h *Handler) DeleteUserAvatar(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.UserIDFromCtx(r.Context())

	if err := h.app.DeleteUserAvatar(r.Context(), chi.URLParam(r, "user_id"), requesterID); err != nil {
		response.ResponseError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Health обрабатывает GET /health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	health := h.app.Check(r.Context())

	code := http.StatusOK
	if health.Status != avatar.StatusUp {
		code = http.StatusServiceUnavailable
	}

	response.JSON(w, code, health)
}

func formFile(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	file, header, err := r.FormFile("file")
	if err == nil {
		return file, header, nil
	}

	// Ошибки чтения тела запроса пробрасываем как есть, не пытаясь читать его повторно.
	if !errors.Is(err, http.ErrMissingFile) {
		return nil, nil, err
	}

	return r.FormFile("image")
}

func parseSize(r *http.Request) (string, error) {
	size := r.URL.Query().Get("size")

	switch size {
	case "", model.SizeOriginal, model.ThumbnailSmall, model.ThumbnailMedium:
		return size, nil
	default:
		return "", avatar.BadRequestError("Invalid size",
			"Supported sizes: 100x100, 300x300, original")
	}
}

func writeContent(w http.ResponseWriter, r *http.Request, content *avatar.Content) {
	defer func() { _ = content.Body.Close() }()

	etag := strconv.Quote(content.ETag)

	// Клиент уже держит эту версию изображения в кеше.
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", content.MimeType)
	w.Header().Set("Cache-Control", "max-age="+strconv.Itoa(cacheMaxAge))
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, content.Body); err != nil {
		// Заголовки уже отправлены, поэтому ошибку остается только залогировать.
		response.LogError(r, err)
	}
}
