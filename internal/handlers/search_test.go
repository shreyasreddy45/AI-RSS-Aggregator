package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestParseSearchRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/search?q=bitcoin+ai&limit=25&offset=50&sort=relevance&feed_id=123e4567-e89b-12d3-a456-426614174000&from=2024-01-01&to=2024-12-31", nil)

	parsed, err := parseSearchRequest(req)
	if err != nil {
		t.Fatalf("parseSearchRequest() returned error: %v", err)
	}

	if parsed.Query != "bitcoin ai" {
		t.Fatalf("expected query to be normalized, got %q", parsed.Query)
	}
	if parsed.Limit != 25 {
		t.Fatalf("expected limit 25, got %d", parsed.Limit)
	}
	if parsed.Offset != 50 {
		t.Fatalf("expected offset 50, got %d", parsed.Offset)
	}
	if parsed.Sort != "relevance" {
		t.Fatalf("expected sort relevance, got %q", parsed.Sort)
	}
	if parsed.FeedID == "" {
		t.Fatal("expected feed id to be parsed")
	}
	if parsed.From.IsZero() || parsed.To.IsZero() {
		t.Fatal("expected date filters to be parsed")
	}
}

func TestParseSearchRequestRejectsInvalidSort(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/search?q=bitcoin&sort=invalid", nil)

	_, err := parseSearchRequest(req)
	if err == nil {
		t.Fatal("expected invalid sort to be rejected")
	}
}

func TestParseSearchRequestSupportsSemanticMode(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/search?q=bitcoin+ai&mode=hybrid&limit=15", nil)

	parsed, err := parseSearchRequest(req)
	if err != nil {
		t.Fatalf("parseSearchRequest() returned error: %v", err)
	}
	if parsed.Mode != "hybrid" {
		t.Fatalf("expected mode hybrid, got %q", parsed.Mode)
	}
	if parsed.Limit != 15 {
		t.Fatalf("expected limit 15, got %d", parsed.Limit)
	}
}
