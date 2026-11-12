package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestChiURLParamReturnsRouteValue(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/posts/{postID}/summary", func(w http.ResponseWriter, r *http.Request) {
		postID := chi.URLParam(r, "postID")
		if postID == "" {
			t.Fatal("postID path parameter was empty")
		}
		if postID != "1848d67a-5eed-42a6-8db6-28cc5bfde978" {
			t.Fatalf("expected postID to match UUID, got %q", postID)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/posts/1848d67a-5eed-42a6-8db6-28cc5bfde978/summary", nil)
	res := httptest.NewRecorder()

	r.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
}
