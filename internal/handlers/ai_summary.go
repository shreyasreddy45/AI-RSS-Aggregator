package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// GeneratePostSummary generates (or regenerates) the AI summary for a post on
// demand. The work happens in the Python AI service; this handler only checks
// the JWT and the post ID.
//
//	@Summary	Generate post summary
//	@Tags		Posts
//	@Security	BearerAuth
//	@Produce	json
//	@Param		postID	path		string	true	"Post ID"
//	@Success	200		{object}	object
//	@Failure	400		{object}	map[string]string
//	@Failure	404		{object}	map[string]string
//	@Failure	502		{object}	map[string]string
//	@Failure	503		{object}	map[string]string
//	@Router		/posts/{postID}/summary [post]
func GeneratePostSummary(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "postID must be a valid UUID")
			return
		}
		forwardToAI(a, w, r, user, http.MethodPost, "/posts/"+postID.String()+"/summary", nil, nil)
	}
}
