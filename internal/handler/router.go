package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/vrnvgasu/gophprofile/internal/handler/middleware"
)

// NewRouter создает chi-роутер.
func NewRouter(h *Handler, staticDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.UserID)

	r.Get("/health", h.Health)

	r.Route("/api/v1", func(api chi.Router) {
		api.Route("/avatars", func(avatars chi.Router) {
			avatars.Post("/", h.UploadAvatar)
			avatars.Get("/{avatar_id}", h.GetAvatar)
			avatars.Delete("/{avatar_id}", h.DeleteAvatar)
			avatars.Get("/{avatar_id}/metadata", h.GetAvatarMetadata)
		})

		api.Route("/users/{user_id}", func(users chi.Router) {
			users.Get("/avatar", h.GetUserAvatar)
			users.Delete("/avatar", h.DeleteUserAvatar)
			users.Get("/avatars", h.ListUserAvatars)
		})
	})

	r.Route("/web", func(web chi.Router) {
		web.Get("/upload", serveIndex(staticDir))
		web.Post("/upload", h.WebUpload)
		web.Get("/gallery/{user_id}", serveIndex(staticDir))
	})

	mountStatic(r, staticDir)

	return r
}

func mountStatic(r chi.Router, staticDir string) {
	if staticDir == "" {
		return
	}

	if _, err := os.Stat(staticDir); err != nil {
		return
	}

	fileServer := http.FileServer(http.Dir(staticDir))
	index := filepath.Join(staticDir, "index.html")

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		requested := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(requested); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.ServeFile(w, r, index)
	})
}
