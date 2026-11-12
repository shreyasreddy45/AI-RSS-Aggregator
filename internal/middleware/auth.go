package middleware

import (
	"net/http"

	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/auth"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// Auth wraps a handler that requires an authenticated user resolved from
// the API key on the request (Authorization: ApiKey <api_key>).
func Auth(a *app.App, handler app.AuthedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey, err := auth.GetAPIKey(r.Header)
		if err != nil {
			jsonutil.RespondWithError(w, 400, err.Error())
			return
		}
		user, err := a.DB.GetUserByApiKey(r.Context(), apiKey)
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to get user")
			return
		}
		handler(w, r, user)
	}
}
