package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/database"
	"github.com/shreyasreddy45/RSS-server/internal/models"
	serviceauth "github.com/shreyasreddy45/RSS-server/internal/service/auth"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateUser registers a new user account.
//
//	@Summary	Create user
//	@Tags		Users
//	@Accept		json
//	@Produce	json
//	@Param		request	body		handlers.CreateUserRequest	true	"User payload"
//	@Success	201		{object}	models.User
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/users [post]
func CreateUser(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)

		params := CreateUserRequest{}

		err := decoder.Decode(&params)
		if err != nil {
			jsonutil.RespondWithError(w, 400, err.Error())
			return
		}
		hashedPassword, err := serviceauth.HashPassword(params.Password)
		if err != nil {
			jsonutil.RespondWithError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
		user, err := a.DB.CreateUser(r.Context(), database.CreateUserParams{
			ID:           uuid.New(),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Name:         params.Name,
			Email:        params.Email,
			PasswordHash: hashedPassword,
		})
		if err != nil {
			a.Logger.Error(
				"failed creating user",
				"error", err,
			)

			jsonutil.RespondWithError(
				w,
				http.StatusInternalServerError,
				"Internal Server Error",
			)
			return
		}

		jsonutil.ResponseWithJSON(w, http.StatusCreated, user)
	}
}

// GetUsers returns the authenticated user details from the JWT identity.
//
//	@Summary	Get current user
//	@Tags		Users
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	models.User
//	@Failure	500	{object}	map[string]string
//	@Router		/users [get]
func GetUsers(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		jsonutil.ResponseWithJSON(w, 200, user)
	}
}

// GetUsersByApiKey is retained as a compatibility shim for internal API-key usage only.
func GetUsersByApiKey(a *app.App) app.AuthedHandler {
	return GetUsers(a)
}

// GetPostForUser returns posts visible to the authenticated user with pagination and search.
//
//	@Summary	List posts
//	@Tags		Posts
//	@Security	BearerAuth
//	@Produce	json
//	@Param		search	query		string	false	"Search text"
//	@Param		limit	query		int		false	"Page size"
//	@Param		offset	query		int		false	"Offset"
//	@Success	200		{array}		models.Post
//	@Failure	400		{object}	map[string]string
//	@Failure	500		{object}	map[string]string
//	@Router		/posts [get]
func GetPostForUser(a *app.App) app.AuthedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {

		limit := int32(10)
		offset := int32(0)
		search := r.URL.Query().Get("search")

		limitStr := r.URL.Query().Get("limit")
		if limitStr != "" {
			value, err := strconv.Atoi(limitStr)
			if err != nil {
				jsonutil.RespondWithError(w, 400, "Invalid limit")
				return
			}
			limit = int32(value)
		}

		offsetStr := r.URL.Query().Get("offset")
		if offsetStr != "" {
			value, err := strconv.Atoi(offsetStr)
			if err != nil {
				jsonutil.RespondWithError(w, 400, "Invalid offset")
				return
			}
			offset = int32(value)
		}

		ctx := context.Background()

		// The key starts with the "rss-cache:GET:/v1/posts" prefix so that
		// cacheutil.InvalidateFeedAndPostCache also clears it.
		cacheKey := fmt.Sprintf(
			"rss-cache:GET:/v1/posts:user:%s:search:%s:%d:%d",
			user.ID,
			search,
			limit,
			offset,
		)

		// ---------------- Redis GET ----------------

		cachedPosts, err := a.Redis.Get(ctx, cacheKey).Result()
		if err == nil {
			a.Logger.Info("Cache Hit", "key", cacheKey)
			if err == redis.Nil {
				a.Logger.Info("Cache Miss", "key", cacheKey)
			}

			var posts []models.Post

			err = json.Unmarshal([]byte(cachedPosts), &posts)
			if err == nil {
				jsonutil.ResponseWithJSON(w, 200, posts)
				return
			}
		}

		if err != nil && err != redis.Nil {
			a.Logger.Error("Redis GET failed", "error", err)
		}

		// ---------------- PostgreSQL ----------------

		posts, err := a.DB.GetPostsForUser(r.Context(), database.GetPostsForUserParams{
			UserID:     user.ID,
			Search:     search,
			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			jsonutil.RespondWithError(w, 500, "Failed to get posts")
			return
		}

		response := models.DatabasePostsToPosts(posts)

		// ---------------- Redis SET ----------------

		data, err := json.Marshal(response)
		if err == nil {
			err = a.Redis.Set(ctx, cacheKey, data, 5*time.Minute).Err()
			if err == nil {
				a.Logger.Info("Cached response", "key", cacheKey)
			}
			if err != nil {
				a.Logger.Error("Redis SET failed", "error", err)
			}
		}

		jsonutil.ResponseWithJSON(w, 200, response)
	}
}
