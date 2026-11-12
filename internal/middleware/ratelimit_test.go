package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitPerClientKey(t *testing.T) {
	rateLimitersMu.Lock()
	rateLimiters = make(map[string]*clientLimiter)
	rateLimitersMu.Unlock()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := RateLimit(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	testCases := []struct {
		name         string
		userID       string
		remoteAddr   string
		requestCount int
		expected     int
	}{
		{name: "user-one reaches limit", userID: "user-1", remoteAddr: "10.0.0.1:1234", requestCount: 21, expected: http.StatusTooManyRequests},
		{name: "user-two remains allowed", userID: "user-2", remoteAddr: "10.0.0.2:1234", requestCount: 1, expected: http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var lastStatus int
			for i := 0; i < tc.requestCount; i++ {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req = req.WithContext(context.WithValue(req.Context(), "userID", tc.userID))
				req.RemoteAddr = tc.remoteAddr
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				lastStatus = res.Code
			}
			if lastStatus != tc.expected {
				t.Fatalf("expected status %d for %s, got %d", tc.expected, tc.name, lastStatus)
			}
		})
	}

	if got := len(rateLimiters); got != 2 {
		t.Fatalf("expected two limiter entries for separate keys, got %d", got)
	}
}
