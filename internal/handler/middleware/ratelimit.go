package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/vrnvgasu/gophprofile/internal/handler/response"
)

const (
	visitorTTL    = 10 * time.Minute
	sweepInterval = time.Minute
)

// RateLimiter ограничивает частоту запросов по ключу клиента.
type RateLimiter struct {
	limit rate.Limit
	burst int

	mu        sync.Mutex
	visitors  map[string]*visitor
	lastSweep time.Time
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	if rps <= 0 {
		return nil
	}

	if burst < 1 {
		burst = 1
	}

	return &RateLimiter{
		limit:     rate.Limit(rps),
		burst:     burst,
		visitors:  make(map[string]*visitor),
		lastSweep: time.Now(),
	}
}

// Middleware отклоняет запросы сверх лимита с кодом 429.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			response.JSON(w, http.StatusTooManyRequests, response.Error{
				Error:   "Too many requests",
				Details: "Rate limit exceeded, try again later",
			})

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	if now.Sub(l.lastSweep) > sweepInterval {
		l.sweep(now)
	}

	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[key] = v
	}

	v.lastSeen = now

	return v.limiter.Allow()
}

// sweep вызывается под уже взятым замком.
func (l *RateLimiter) sweep(now time.Time) {
	for key, v := range l.visitors {
		if now.Sub(v.lastSeen) > visitorTTL {
			delete(l.visitors, key)
		}
	}

	l.lastSweep = now
}

// clientIP определяет клиента по адресу. За ingress-контроллером реальный
// адрес приходит заголовком, поэтому он в приоритете: сам сервис наружу
// не смотрит, и подделать заголовок в обход контроллера некому.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")

		return strings.TrimSpace(first)
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
