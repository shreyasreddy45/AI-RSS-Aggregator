package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func RequestID(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		id := uuid.NewString()

		ctx := context.WithValue(r.Context(), requestIDKey, id)

		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", id)

		next.ServeHTTP(w, r)
	})
}
