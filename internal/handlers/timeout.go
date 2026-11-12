package handlers

import (
	"context"
	"net/http"
	"time"
)

func requestContextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return r.Context(), func() {}
	}
	return context.WithTimeout(r.Context(), timeout)
}
