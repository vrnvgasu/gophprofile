package middleware

import (
	"net/http"
	"time"

	"github.com/vrnvgasu/gophprofile/internal/logger"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.size += size

	return size, err
}

// Logger логирует метод, путь, код ответа, размер тела и длительность обработки запроса.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		logger.Log.Infow("http request",
			"method", r.Method,
			"uri", r.RequestURI,
			"status", rw.status,
			"size", rw.size,
			"duration", time.Since(start).String(),
		)
	})
}
