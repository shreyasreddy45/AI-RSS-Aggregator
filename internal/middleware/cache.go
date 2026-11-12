package middleware

import (
	"net/http"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	cacheutil "github.com/shreyasreddy45/RSS-server/internal/redis"
)

func Cache(a *app.App, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !cacheutil.ShouldCache(r) {
				next.ServeHTTP(w, r)
				return
			}

			cacheKey := cacheutil.CacheKey(r)
			cached, err := a.Redis.Get(r.Context(), cacheKey).Result()
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(cached))
				return
			}
			if err != nil && err != redislib.Nil {
				a.Logger.Error("Redis GET failed", "error", err)
			}

			responseRecorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK, body: strings.Builder{}}
			next.ServeHTTP(responseRecorder, r)

			if responseRecorder.statusCode < http.StatusOK || responseRecorder.statusCode >= http.StatusMultipleChoices {
				return
			}

			payload := responseRecorder.body.String()
			if payload == "" {
				return
			}

			if err := a.Redis.Set(r.Context(), cacheKey, payload, ttl).Err(); err != nil {
				a.Logger.Error("Redis SET failed", "error", err)
			}
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       strings.Builder
}

func (rr *responseRecorder) Header() http.Header {
	return rr.ResponseWriter.Header()
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	_, _ = rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}
