package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type searchRequest struct {
	Query  string
	Limit  int
	Offset int
	Sort   string
	Mode   string
	FeedID string
	From   time.Time
	To     time.Time
}

type searchResult struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PublishedAt time.Time `json:"published_at"`
	URL         string    `json:"url"`
	FeedID      uuid.UUID `json:"feed_id"`
	Rank        float64   `json:"rank"`
}

type searchResponse struct {
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	Sort    string         `json:"sort"`
	Mode    string         `json:"mode"`
	Results []searchResult `json:"results"`
}

func parseSearchRequest(r *http.Request) (searchRequest, error) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return searchRequest{}, errors.New("q is required")
	}
	query = strings.Join(strings.Fields(query), " ")
	if len(query) > 200 {
		return searchRequest{}, errors.New("q must be 200 characters or less")
	}

	limit := 20
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return searchRequest{}, errors.New("limit must be an integer between 1 and 100")
		}
		limit = n
	}

	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return searchRequest{}, errors.New("offset must be a non-negative integer")
		}
		offset = n
	}

	sortKey := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	switch sortKey {
	case "", "relevance":
		sortKey = "relevance"
	case "newest", "oldest":
	default:
		return searchRequest{}, errors.New("sort must be one of: relevance, newest, oldest")
	}

	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	switch mode {
	case "", "text", "semantic", "hybrid":
	default:
		return searchRequest{}, errors.New("mode must be one of: text, semantic, hybrid")
	}
	if mode == "" {
		mode = "text"
	}

	feedID := strings.TrimSpace(r.URL.Query().Get("feed_id"))
	if feedID != "" {
		if _, err := uuid.Parse(feedID); err != nil {
			return searchRequest{}, errors.New("feed_id must be a valid UUID")
		}
	}

	from, err := parseOptionalDate(r.URL.Query().Get("from"))
	if err != nil {
		return searchRequest{}, err
	}
	to, err := parseOptionalDate(r.URL.Query().Get("to"))
	if err != nil {
		return searchRequest{}, err
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return searchRequest{}, errors.New("from must be before or equal to to")
	}

	return searchRequest{
		Query:  query,
		Limit:  limit,
		Offset: offset,
		Sort:   sortKey,
		Mode:   mode,
		FeedID: feedID,
		From:   from,
		To:     to,
	}, nil
}

func parseOptionalDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD", value)
	}
	return parsed, nil
}

// SearchPosts runs text, semantic, or hybrid search across the user's followed feeds.
//
//	@Summary	Search posts
//	@Tags		Search
//	@Security	BearerAuth
//	@Produce	json
//	@Param		q		query		string	true	"Search query"
//	@Param		limit	query		int		false	"Limit"
//	@Param		offset	query		int		false	"Offset"
//	@Param		sort	query		string	false	"Sort mode (relevance, newest, oldest)"
//	@Param		mode	query		string	false	"Search mode (text, semantic, hybrid)"
//	@Param		feed_id	query		string	false	"Optional feed UUID"
//	@Success	200		{object}	handlers.searchResponse
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/search [get]
func SearchPosts(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		params, err := parseSearchRequest(r)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		switch params.Mode {
		case "", "text":
			handleTextSearch(w, r, a, user, params)
		case "semantic", "hybrid":
			handleAISearch(w, r, a, user)
		default:
			jsonutil.RespondWithError(w, http.StatusBadRequest, "mode must be one of: text, semantic, hybrid")
		}
	}
}

func handleTextSearch(w http.ResponseWriter, r *http.Request, a *app.App, user database.User, params searchRequest) {
	whereClause, countArgs, queryArgs := buildSearchClauses(user.ID, params)
	dbCtx, dbCancel := requestContextWithTimeout(r, a.Config.DBQueryTimeout)
	defer dbCancel()

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM posts p
		JOIN feed_follows ff ON ff.feed_id = p.feed_id
		%s
	`, whereClause)

	countRow := a.SQLDB.QueryRowContext(dbCtx, countQuery, countArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to count search results")
		return
	}

	orderBy := buildOrderBy(params.Sort)
	searchQuery := fmt.Sprintf(`
		SELECT
			p.id,
			p.title,
			p.description,
			p.created_at,
			p.updated_at,
			p.published_at,
			p.url,
			p.feed_id,
			ts_rank_cd(p.search_document, websearch_to_tsquery('english', $2)) AS rank
		FROM posts p
		JOIN feed_follows ff ON ff.feed_id = p.feed_id
		%s
		%s
		LIMIT $%d
		OFFSET $%d
	`, whereClause, orderBy, len(queryArgs)+1, len(queryArgs)+2)
	queryArgs = append(queryArgs, params.Limit, params.Offset)

	rows, err := a.SQLDB.QueryContext(dbCtx, searchQuery, queryArgs...)
	if err != nil {
		a.Logger.Error("Search query failed", "error", err, "query", params.Query)
		jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to search posts")
		return
	}
	defer rows.Close()

	results, err := scanSearchRows(rows)
	if err != nil {
		a.Logger.Error("Scan search rows failed", "error", err)
		jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to parse search results")
		return
	}

	jsonutil.ResponseWithJSON(w, http.StatusOK, searchResponse{
		Query:   params.Query,
		Total:   total,
		Limit:   params.Limit,
		Offset:  params.Offset,
		Sort:    params.Sort,
		Mode:    params.Mode,
		Results: results,
	})
}

// handleAISearch forwards semantic and hybrid searches to the Python AI
// service, which owns embedding generation and vector ranking. The query string
// has already been validated by parseSearchRequest.
func handleAISearch(w http.ResponseWriter, r *http.Request, a *app.App, user database.User) {
	forwardToAI(a, w, r, user, http.MethodGet, "/search", r.URL.Query(), nil)
}

func scanSearchRows(rows *sql.Rows) ([]searchResult, error) {
	results := make([]searchResult, 0)
	for rows.Next() {
		var (
			id          uuid.UUID
			title       string
			description sql.NullString
			createdAt   time.Time
			updatedAt   time.Time
			publishedAt time.Time
			url         string
			feedID      uuid.UUID
			rank        float64
		)

		if err := rows.Scan(&id, &title, &description, &createdAt, &updatedAt, &publishedAt, &url, &feedID, &rank); err != nil {
			return nil, err
		}

		result := searchResult{
			ID:          id,
			Title:       title,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			PublishedAt: publishedAt,
			URL:         url,
			FeedID:      feedID,
			Rank:        rank,
		}
		if description.Valid {
			desc := description.String
			result.Description = &desc
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func buildSearchClauses(userID uuid.UUID, params searchRequest) (string, []any, []any) {
	args := []any{userID, params.Query}
	where := "WHERE ff.user_id = $1 AND p.search_document @@ websearch_to_tsquery('english', $2)"
	placeholder := 3

	if params.FeedID != "" {
		where += fmt.Sprintf(" AND p.feed_id = $%d", placeholder)
		args = append(args, params.FeedID)
		placeholder++
	}
	if !params.From.IsZero() {
		where += fmt.Sprintf(" AND p.published_at >= $%d", placeholder)
		args = append(args, params.From)
		placeholder++
	}
	if !params.To.IsZero() {
		where += fmt.Sprintf(" AND p.published_at <= $%d", placeholder)
		args = append(args, params.To.Add(24*time.Hour))
		placeholder++
	}
	return where, args, append([]any{}, args...)
}

func buildOrderBy(sort string) string {
	switch sort {
	case "newest":
		return "ORDER BY p.published_at DESC"
	case "oldest":
		return "ORDER BY p.published_at ASC"
	case "relevance":
		return "ORDER BY rank DESC, p.published_at DESC"
	default:
		return "ORDER BY rank DESC, p.published_at DESC"
	}
}
