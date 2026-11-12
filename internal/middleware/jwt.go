package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	serviceauth "github.com/shreyasreddy45/RSS-server/internal/service/auth"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// JWT wraps a handler that requires an authenticated user resolved from a
// Bearer JWT on the request.
func JWT(a *app.App, handler app.AuthedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}

		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "Invalid Authorization header")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, prefix)
		claims, err := serviceauth.ValidateJWT(tokenString, a.Config.JWTSecret)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "Invalid user id")
			return
		}

		user, err := a.DB.GetUserByID(r.Context(), userID)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "User not found")
			return
		}

		ctx := context.WithValue(r.Context(), "userID", user.ID.String())
		handler(w, r.WithContext(ctx), user)
	}
}
