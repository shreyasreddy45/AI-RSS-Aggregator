package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	"github.com/shreyasreddy45/RSS-server/internal/models"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type CreateFeedFollowRequest struct {
	FeedID uuid.UUID `json:"feed_id"`
}

// CreateFeedFollow follows a feed for the authenticated user.
//
//	@Summary	Follow feed
//	@Tags		Feed Follows
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.CreateFeedFollowRequest	true	"Follow payload"
//	@Success	201		{object}	models.FeedFollow
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/feedfollows [post]
func CreateFeedFollow(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		decoder := json.NewDecoder(r.Body)
		params := CreateFeedFollowRequest{}
		err := decoder.Decode(&params)
		if err != nil {
			jsonutil.RespondWithError(w, 400, err.Error())
			return
		}
		feedfollow, err := a.DB.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			UserID:    user.ID,
			FeedID:    params.FeedID,
		})
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to create feed follow")
			return
		}
		invalidateContentCache(a, r)

		jsonutil.ResponseWithJSON(w, 201, feedfollow)
	}
}

// GetFeedFollows returns the feeds followed by the authenticated user.
//
//	@Summary	List followed feeds
//	@Tags		Feed Follows
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{array}		models.FeedFollow
//	@Failure	500	{object}	map[string]string
//	@Router		/feedfollows [get]
func GetFeedFollows(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		feedfollows, err := a.DB.GetFeedFollows(r.Context(), user.ID)
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to get feed follows")
			return
		}
		jsonutil.ResponseWithJSON(w, 200, models.DatabaseFeedFollowsToFeedFollows(feedfollows))
	}
}

// DeleteFeedFollow removes a feed follow for the authenticated user.
//
//	@Summary	Unfollow feed
//	@Tags		Feed Follows
//	@Security	BearerAuth
//	@Produce	json
//	@Param		feedfollowID	path		string	true	"Feed follow ID"
//	@Success	200				{object}	map[string]string
//	@Failure	400				{object}	map[string]string
//	@Failure	500				{object}	map[string]string
//	@Router		/feedfollows/{feedfollowID} [delete]
func DeleteFeedFollow(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		feedFollowIDstr := chi.URLParam(r, "feedfollowID")
		feedFollowID, err := uuid.Parse(feedFollowIDstr)
		if err != nil {
			jsonutil.RespondWithError(w, 400, "Invalid feed follow ID")
			return
		}
		err = a.DB.DeleteFeedFollow(r.Context(), database.DeleteFeedFollowParams{
			ID:     feedFollowID,
			UserID: user.ID,
		})
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to delete feed follow")
			return
		}
		invalidateContentCache(a, r)
		jsonutil.ResponseWithJSON(w, 200, map[string]string{"message": "Feed follow deleted successfully"})
	}
}
