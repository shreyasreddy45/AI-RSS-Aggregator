package middleware

import (
	"net/http"

	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

func RequireRole(
	a *app.App,
	role string,
	handler app.AuthedHandler,
) http.HandlerFunc {

	return JWT(
		a,
		func(w http.ResponseWriter, r *http.Request, user database.User) {

			if user.Role.String != role {
				jsonutil.RespondWithError(
					w,
					http.StatusForbidden,
					"Forbidden",
				)
				return
			}

			handler(w, r, user)
		},
	)
}
