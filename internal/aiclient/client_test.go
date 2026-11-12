package aiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestDoSendsAuthHeadersAndQuery(t *testing.T) {
	userID := uuid.New()
	var gotToken, gotUser, gotPath, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		gotUser = r.Header.Get("X-User-ID")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "s3cret")
	q := url.Values{}
	q.Set("q", "go lang")

	resp, err := c.Do(context.Background(), http.MethodGet, "/search", userID, q, nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if resp.Status != http.StatusOK || string(resp.Body) != `{"ok":true}` {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if gotToken != "s3cret" {
		t.Fatalf("expected token header, got %q", gotToken)
	}
	if gotUser != userID.String() {
		t.Fatalf("expected user header %s, got %q", userID, gotUser)
	}
	if gotPath != "/search" || gotQuery != "q=go+lang" {
		t.Fatalf("unexpected target %q?%q", gotPath, gotQuery)
	}
}

func TestDoReportsUnavailableWhenServiceIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	c := New(url, "")
	_, err := c.Do(context.Background(), http.MethodGet, "/health", uuid.Nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error when the service is unreachable")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestProcessPostSurfacesUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadGateway)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	if err := c.ProcessPost(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected ProcessPost to fail on a non-200 response")
	}
}

func TestPendingPostsDecodesIDs(t *testing.T) {
	id := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "50" {
			t.Errorf("expected limit=50, got %q", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{"post_ids":["` + id.String() + `"]}`))
	}))
	defer srv.Close()

	ids, err := New(srv.URL, "").PendingPosts(context.Background(), 50)
	if err != nil {
		t.Fatalf("PendingPosts returned error: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
