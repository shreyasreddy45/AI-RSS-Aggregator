package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
)

func registerFeedRoutes(r chi.Router, a *app.App) {
	r.Post("/feeds", middleware.JWT(a, handlers.CreateFeed(a)))
	r.Get("/feeds", func(w http.ResponseWriter, r *http.Request) {
		middleware.Cache(a, a.Config.CacheTTL)(http.HandlerFunc(middleware.JWT(a, handlers.GetFeeds(a)))).ServeHTTP(w, r)
	})

	r.Post("/feedfollows", middleware.JWT(a, handlers.CreateFeedFollow(a)))
	r.Get("/feedfollows", middleware.JWT(a, handlers.GetFeedFollows(a)))
	r.Delete("/feedfollows/{feedfollowID}", middleware.JWT(a, handlers.DeleteFeedFollow(a)))
}
