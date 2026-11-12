package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type postSummaryResponse struct {
	ID        uuid.UUID `json:"id"`
	PostID    uuid.UUID `json:"post_id"`
	Summary   string    `json:"summary"`
	Model     string    `json:"model"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// GetPostSummary returns the stored AI summary for a post.
//
//	@Summary	Get post summary
//	@Tags		Posts
//	@Security	BearerAuth
//	@Produce	json
//	@Param		postID	path		string	true	"Post ID"
//	@Success	200		{object}	handlers.postSummaryResponse
//	@Failure	400		{object}	map[string]string
//	@Failure	404		{object}	map[string]string
//	@Router		/posts/{postID}/summary [get]
func GetPostSummary(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		postIDRaw := chi.URLParam(r, "postID")
		if postIDRaw == "" {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "postID is required")
			return
		}

		postID, err := uuid.Parse(postIDRaw)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "postID must be a valid UUID")
			return
		}

		dbCtx, dbCancel := requestContextWithTimeout(r, a.Config.DBQueryTimeout)
		defer dbCancel()
		if _, err := a.DB.GetPostByIDForUser(dbCtx, database.GetPostByIDForUserParams{
			UserID: user.ID,
			ID:     postID,
		}); err != nil {
			jsonutil.RespondWithError(w, http.StatusNotFound, "Summary not found")
			return
		}

		summary, err := a.DB.GetPostSummaryByPostID(dbCtx, postID)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusNotFound, "Summary not found")
			return
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, postSummaryResponse{
			ID:        summary.ID,
			PostID:    summary.PostID,
			Summary:   summary.Summary,
			Model:     summary.Model,
			CreatedAt: summary.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: summary.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}
