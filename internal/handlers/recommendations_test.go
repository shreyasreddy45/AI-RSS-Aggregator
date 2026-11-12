package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeriveTags(t *testing.T) {
	tags := deriveTags("AI startup news and machine learning for developers in 2026")
	if len(tags) == 0 {
		t.Fatal("expected tags to be derived from the title")
	}
	if len(tags) > 5 {
		t.Fatalf("expected at most 5 tags, got %d", len(tags))
	}
}

func TestScoreRecommendation(t *testing.T) {
	postID := uuid.New()
	feedID := uuid.New()
	item := recommendationItem{
		ID:           postID,
		FeedID:       feedID,
		Title:        "AI startup funding and machine learning",
		Description:  testStringPtr("Artificial intelligence investing and startup growth in the developer ecosystem"),
		FeedName:     "AI Startup News",
		PublishedAt:  time.Now().Add(-2 * time.Hour),
		Category:     "ai startup news",
		Tags:         []string{"ai", "startup", "machine", "learning"},
		Trending:     12,
		RecentScrape: time.Now().Add(-30 * time.Minute),
	}

	score := scoreRecommendation(item)
	if score <= 0 {
		t.Fatalf("expected a positive recommendation score, got %f", score)
	}
}

func testStringPtr(s string) *string {
	return &s
}
