package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestContextWithTimeoutCancelsSlowDependency(t *testing.T) {
	timeout := 200 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			http.Error(w, r.Context().Err().Error(), http.StatusRequestTimeout)
			return
		}
		http.Error(w, "context cancelled", http.StatusRequestTimeout)
	}))
	defer server.Close()

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, server.URL, nil)
	ctx, cancel := requestContextWithTimeout(req, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext returned error: %v", err)
	}

	_, err = http.DefaultClient.Do(httpReq)
	if err == nil {
		t.Fatal("expected timeout error from slow dependency, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected a timeout error, got %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > timeout+500*time.Millisecond {
		t.Fatalf("request took too long to timeout: %s > %s", elapsed, timeout+500*time.Millisecond)
	}
}
