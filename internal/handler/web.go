package handler

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/vrnvgasu/gophprofile/internal/handler/middleware"
	"github.com/vrnvgasu/gophprofile/internal/handler/response"
	"github.com/vrnvgasu/gophprofile/internal/service/avatar"
)

// WebUpload обрабатывает POST /web/upload — отправку формы загрузки.
func (h *Handler) WebUpload(w http.ResponseWriter, r *http.Request) {
	data, fileName, err := h.readUploadedFile(w, r)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	userID, err := webUserID(r)
	if err != nil {
		response.ResponseError(w, r, err)
		return
	}

	if _, err = h.app.Upload(r.Context(), userID, fileName, data); err != nil {
		response.ResponseError(w, r, err)
		return
	}

	http.Redirect(w, r, galleryPath(userID), http.StatusSeeOther)
}

func webUserID(r *http.Request) (string, error) {
	userID := r.FormValue("userId")
	if userID == "" {
		userID = r.FormValue("user_id")
	}
	if userID == "" {
		userID = middleware.UserIDFromCtx(r.Context())
	}

	if userID == "" {
		return "", avatar.UnauthorizedError()
	}

	if !middleware.IsValidUserID(userID) {
		return "", avatar.BadRequestError("Invalid user id", "User id contains unsupported characters")
	}

	return userID, nil
}

func galleryPath(userID string) string {
	return "/web/gallery/" + url.PathEscape(userID)
}

func serveIndex(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if staticDir == "" {
			http.NotFound(w, r)
			return
		}

		index := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, index)
	}
}
