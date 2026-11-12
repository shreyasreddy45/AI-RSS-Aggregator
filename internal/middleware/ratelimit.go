package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
	"golang.org/x/time/rate"
)

const (
	rateLimitPerSecond = 10
	rateLimitBurst     = 20
	rateLimitIdleTTL   = 10 * time.Minute
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	rateLimitersMu sync.Mutex
	rateLimiters   = make(map[string]*clientLimiter)
)

// RateLimit was previously a plain middleware that closed over a
// package-level `logger` global in main.go; the logger is now passed in
// explicitly so the middleware package has no hidden dependencies.
func RateLimit(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientRateLimitKey(r)
			limiter := getClientLimiter(key)
			if !limiter.Allow() {
				logger.Warn(
					"Rate limit exceeded",
					"key", key,
					"path", r.URL.Path,
					"method", r.Method,
					"remote_addr", r.RemoteAddr,
					"x_forwarded_for", r.Header.Get("X-Forwarded-For"),
				)
				jsonutil.RespondWithError(w, http.StatusTooManyRequests, "Too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getClientLimiter(key string) *rate.Limiter {
	rateLimitersMu.Lock()
	defer rateLimitersMu.Unlock()

	now := time.Now()
	for currentKey, entry := range rateLimiters {
		if now.Sub(entry.lastSeen) > rateLimitIdleTTL {
			delete(rateLimiters, currentKey)
		}
	}

	entry, ok := rateLimiters[key]
	if !ok {
		entry = &clientLimiter{
			limiter:  rate.NewLimiter(rate.Limit(rateLimitPerSecond), rateLimitBurst),
			lastSeen: now,
		}
		rateLimiters[key] = entry
		return entry.limiter
	}

	entry.lastSeen = now
	return entry.limiter
}

func clientRateLimitKey(r *http.Request) string {
	if userID, ok := r.Context().Value("userID").(string); ok && strings.TrimSpace(userID) != "" {
		return "user:" + strings.TrimSpace(userID)
	}

	if ip := clientIP(r); ip != "" {
		return "ip:" + ip
	}

	return "anonymous"
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		for _, value := range strings.Split(forwarded, ",") {
			candidate := strings.TrimSpace(value)
			if candidate != "" {
				return normalizeIP(candidate)
			}
		}
	}

	return normalizeIP(r.RemoteAddr)
}

func normalizeIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}
	return strings.Trim(raw, "[]")
}
