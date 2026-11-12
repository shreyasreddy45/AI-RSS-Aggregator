package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// Recovery, like RateLimit, now takes the logger explicitly instead of
// closing over the old package-level `logger` global in main.go.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				value := recover()
				if value != nil {
					logger.Error(
						"Recovered from panic",
						"panic", value,
						"path", r.URL.Path,
						"method", r.Method,
						"stack", string(debug.Stack()),
					)
					jsonutil.RespondWithError(
						w,
						http.StatusInternalServerError,
						"Internal Server Error",
					)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
