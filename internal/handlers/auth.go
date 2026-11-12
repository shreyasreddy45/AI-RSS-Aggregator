package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/auth"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	serviceauth "github.com/shreyasreddy45/RSS-server/internal/service/auth"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login authenticates a user and returns a JWT access token.
//
//	@Summary	Login
//	@Tags		Auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.LoginRequest	true	"Credentials"
//	@Success	200		{object}	handlers.LoginResponse
//	@Failure	400		{object}	map[string]string
//	@Failure	401		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/login [post]
func Login(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		user, err := a.DB.GetUserByEmail(r.Context(), params.Email)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonutil.RespondWithError(w, http.StatusUnauthorized, "Invalid email or password")
				return
			}

			a.Logger.Error("failed getting user", "error", err)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
			return
		}

		if err := serviceauth.CheckPassword(params.Password, user.PasswordHash); err != nil {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "Invalid email or password")
			return
		}

		token, err := serviceauth.GenerateJWT(
			user.ID.String(),
			user.Email,
			user.Role.String,
			a.Config.JWTSecret,
			a.Config.JWTExpiry,
		)
		if err != nil {
			a.Logger.Error("failed generating jwt", "error", err)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
			return
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, LoginResponse{Token: token})
	}
}

// Refresh issues a new access token from a valid refresh token.
//
//	@Summary	Refresh token
//	@Tags		Auth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.RefreshRequest	true	"Refresh token payload"
//	@Success	200		{object}	map[string]string
//	@Failure	400		{object}	map[string]string
//	@Failure	401		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/refresh [post]
func Refresh(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "bad request")
			return
		}

		hash := auth.HashRefreshToken(req.RefreshToken)
		token, err := a.DB.GetRefreshToken(r.Context(), hash)
		if err != nil || token.Revoked || token.ExpiresAt.Before(time.Now()) {
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}

		user, err := a.DB.GetUserByID(r.Context(), token.UserID)
		if err != nil {
			a.Logger.Error("failed getting user for refresh token", "error", err)
			jsonutil.RespondWithError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}

		access, err := serviceauth.GenerateJWT(
			user.ID.String(),
			user.Email,
			user.Role.String,
			a.Config.JWTSecret,
			a.Config.JWTExpiry,
		)
		if err != nil {
			a.Logger.Error("failed generating jwt for refresh", "error", err)
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "jwt failed")
			return
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, map[string]string{"access_token": access})
	}
}

// Logout invalidates a refresh token and logs the user out.
//
//	@Summary	Logout
//	@Tags		Auth
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.RefreshRequest	true	"Refresh token payload"
//	@Success	200		{object}	map[string]string
//	@Failure	400		{object}	map[string]string
//	@Failure	401		{object}	map[string]string
//	@Router		/logout [post]
func Logout(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		var req RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonutil.RespondWithError(w, http.StatusBadRequest, "bad request")
			return
		}

		hash := auth.HashRefreshToken(req.RefreshToken)
		if err := a.DB.RevokeRefreshToken(r.Context(), hash); err != nil {
			a.Logger.Error("failed revoking refresh token", "error", err)
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
	}
}
