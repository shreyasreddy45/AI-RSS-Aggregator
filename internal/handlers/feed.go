package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	"github.com/shreyasreddy45/RSS-server/internal/models"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type CreateFeedRequest struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}

// CreateFeed creates or reuses a feed and subscribes the authenticated user.
//
//	@Summary	Create or reuse feed and follow it
//	@Tags		Feeds
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.CreateFeedRequest	true	"Feed payload"
//	@Success	201		{object}	models.Feed
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/feeds [post]
func CreateFeed(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		decoder := json.NewDecoder(r.Body)
		params := CreateFeedRequest{}
		err := decoder.Decode(&params)
		if err != nil {
			jsonutil.RespondWithError(w, 400, err.Error())
			return
		}
		params.Name, params.Url, err = validateFeedInput(params.Name, params.Url)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		tx, err := a.SQLDB.BeginTx(r.Context(), nil)
		if err != nil {
			a.Logger.Error("failed to begin feed subscription", "error", err, "url", params.Url)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to create feed")
			return
		}
		defer tx.Rollback()

		queries := a.DB.WithTx(tx)
		now := time.Now()
		feed, err := queries.CreateFeed(r.Context(), database.CreateFeedParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			Name:      params.Name,
			Url:       params.Url,
			UserID:    user.ID,
		})
		if err != nil {
			a.Logger.Error("failed to create feed", "error", err, "url", params.Url)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to create feed")
			return
		}
		_, err = queries.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			UserID:    user.ID,
			FeedID:    feed.ID,
		})
		if err != nil {
			a.Logger.Error("failed to subscribe to feed", "error", err, "feed_id", feed.ID, "user_id", user.ID)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to create feed")
			return
		}
		if err := tx.Commit(); err != nil {
			a.Logger.Error("failed to commit feed subscription", "error", err, "feed_id", feed.ID, "user_id", user.ID)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to create feed")
			return
		}
		invalidateContentCache(a, r)

		jsonutil.ResponseWithJSON(w, 201, models.DatabaseFeedToFeed(feed))
	}
}

func validateFeedInput(name, rawURL string) (string, string, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)

	if name == "" {
		return name, rawURL, errors.New("name is required")
	}
	if utf8.RuneCountInString(name) > 100 {
		return name, rawURL, errors.New("name must be at most 100 characters")
	}
	if rawURL == "" {
		return name, rawURL, errors.New("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || parsed.Host == "" {
		return name, rawURL, errors.New("url must be a valid http or https address, e.g. https://feeds.bbci.co.uk/news/technology/rss.xml")
	}

	return name, rawURL, nil
}

// GetFeeds lists all feeds in the system.
//
//	@Summary	List feeds
//	@Tags		Feeds
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{array}		models.Feed
//	@Failure	500	{object}	map[string]string
//	@Router		/feeds [get]
func GetFeeds(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		feeds, err := a.DB.GetFeeds(r.Context())
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to get feeds")
			return
		}
		jsonutil.ResponseWithJSON(w, 200, models.DatabaseFeedsToFeeds(feeds))
	}
}
