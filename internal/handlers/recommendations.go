package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type recommendationRequest struct {
	Limit  int
	Offset int
	FeedID string
}

type recommendationItem struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	URL          string    `json:"url"`
	FeedID       uuid.UUID `json:"feed_id"`
	FeedName     string    `json:"feed_name"`
	PublishedAt  time.Time `json:"published_at"`
	Category     string    `json:"category"`
	Tags         []string  `json:"tags"`
	Trending     int       `json:"trending"`
	RecentScrape time.Time `json:"recent_scrape"`
	Score        float64   `json:"score"`
}

type recommendationsResponse struct {
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
	Items   []recommendationItem `json:"items"`
	Total   int                  `json:"total"`
	Source  string               `json:"source"`
	Message string               `json:"message,omitempty"`
}

// GetRecommendations returns ranked content recommendations for the authenticated user.
//
//	@Summary	Get recommendations
//	@Tags		Recommendations
//	@Security	BearerAuth
//	@Produce	json
//	@Param		limit	query		int		false	"Limit"
//	@Param		offset	query		int		false	"Offset"
//	@Param		feed_id	query		string	false	"Optional feed UUID"
//	@Success	200		{object}	handlers.recommendationsResponse
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/recommendations [get]
func GetRecommendations(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		params, err := parseRecommendationRequest(r)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		posts, err := a.DB.GetPostsForUser(r.Context(), database.GetPostsForUserParams{
			UserID:     user.ID,
			Search:     "",
			PageLimit:  100,
			PageOffset: 0,
		})
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to load recommendations")
			return
		}

		items := buildRecommendationItems(posts, user.ID)
		if len(items) == 0 {
			jsonutil.ResponseWithJSON(w, http.StatusOK, recommendationsResponse{
				Limit:   params.Limit,
				Offset:  params.Offset,
				Items:   []recommendationItem{},
				Total:   0,
				Source:  "content",
				Message: "No articles available for recommendations",
			})
			return
		}

		sort.Slice(items, func(i, j int) bool {
			return items[i].Score > items[j].Score
		})

		start := min(params.Offset, len(items))
		end := min(start+params.Limit, len(items))
		page := items[start:end]

		response := recommendationsResponse{
			Limit:  params.Limit,
			Offset: params.Offset,
			Items:  page,
			Total:  len(items),
			Source: "content",
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, response)
	}
}

func parseRecommendationRequest(r *http.Request) (recommendationRequest, error) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return recommendationRequest{}, fmt.Errorf("limit must be an integer between 1 and 100")
		}
		limit = n
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return recommendationRequest{}, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = n
	}

	feedID := strings.TrimSpace(r.URL.Query().Get("feed_id"))
	if feedID != "" {
		if _, err := uuid.Parse(feedID); err != nil {
			return recommendationRequest{}, fmt.Errorf("feed_id must be a valid UUID")
		}
	}

	return recommendationRequest{
		Limit:  limit,
		Offset: offset,
		FeedID: feedID,
	}, nil
}

func buildRecommendationItems(posts []database.Post, userID uuid.UUID) []recommendationItem {
	items := make([]recommendationItem, 0, len(posts))
	for _, post := range posts {
		feedName := deriveFeedName(post.FeedID.String())
		category := deriveCategory(post.Title, post.Description.String)
		tags := deriveTags(post.Title + " " + post.Description.String)
		trending := deriveTrendingScore(post.Title)
		recentScrape := time.Now().Add(-time.Duration(len(posts))*time.Minute + 5*time.Minute)
		item := recommendationItem{
			ID:           post.ID,
			Title:        post.Title,
			Description:  stringPtr(post.Description.String),
			URL:          post.Url,
			FeedID:       post.FeedID,
			FeedName:     feedName,
			PublishedAt:  post.PublishedAt,
			Category:     category,
			Tags:         tags,
			Trending:     trending,
			RecentScrape: recentScrape,
		}
		item.Score = scoreRecommendation(item)
		items = append(items, item)
	}
	return items
}

func deriveCategory(title, description string) string {
	text := strings.ToLower(strings.TrimSpace(title + " " + description))
	if strings.Contains(text, "ai") || strings.Contains(text, "machine") || strings.Contains(text, "learning") {
		return "ai"
	}
	if strings.Contains(text, "startup") || strings.Contains(text, "funding") || strings.Contains(text, "venture") {
		return "startup"
	}
	if strings.Contains(text, "security") || strings.Contains(text, "cloud") || strings.Contains(text, "devops") {
		return "engineering"
	}
	if strings.Contains(text, "design") || strings.Contains(text, "ux") || strings.Contains(text, "product") {
		return "design"
	}
	return "general"
}

func deriveTags(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]struct{})
	tags := make([]string, 0, 5)
	for _, word := range words {
		word = strings.Trim(word, "[](){}.,:;!?\"'`/\\")
		if len(word) <= 3 || word == "the" || word == "and" || word == "for" || word == "with" {
			continue
		}
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}
		tags = append(tags, word)
		if len(tags) == 5 {
			break
		}
	}
	return tags
}

func deriveTrendingScore(title string) int {
	text := strings.ToLower(title)
	score := 0
	for _, token := range []string{"ai", "startup", "launch", "release", "funding", "security", "cloud", "developer", "news"} {
		if strings.Contains(text, token) {
			score++
		}
	}
	return score
}

func deriveFeedName(_ string) string {
	return "Featured Feed"
}

func scoreRecommendation(item recommendationItem) float64 {
	categoryBoost := 0.0
	switch item.Category {
	case "ai":
		categoryBoost = 2.5
	case "startup":
		categoryBoost = 2.1
	case "engineering":
		categoryBoost = 1.8
	case "design":
		categoryBoost = 1.5
	default:
		categoryBoost = 0.8
	}

	tagBoost := float64(len(item.Tags)) * 0.8
	trendingBoost := float64(item.Trending) * 1.2
	recencyBoost := 0.0
	if !item.RecentScrape.IsZero() {
		hoursAgo := time.Since(item.RecentScrape).Hours()
		recencyBoost = math.Max(0, 6.0-hoursAgo) * 0.6
	}

	ageHours := time.Since(item.PublishedAt).Hours()
	decay := math.Max(0, 1.0-(ageHours/168.0))
	return (categoryBoost + tagBoost + trendingBoost + recencyBoost) * (0.5 + decay)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
