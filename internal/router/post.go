package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
)

func registerPostRoutes(r chi.Router, a *app.App) {
	// GetPostForUser caches per user itself. It must not be wrapped in the
	// generic middleware.Cache: that middleware runs before JWT resolves the
	// user, so its cache key would be shared by every user.
	r.Get("/posts", middleware.JWT(a, handlers.GetPostForUser(a)))
	r.Get("/posts/{postID}/summary", middleware.JWT(a, handlers.GetPostSummary(a)))
	r.Post("/posts/{postID}/summary", middleware.JWT(a, handlers.GeneratePostSummary(a)))

	// AI endpoints: authenticated here, executed by the Python AI service.
	r.Post("/posts/{postID}/embeddings", middleware.JWT(a, handlers.GeneratePostEmbedding(a)))
	r.Get("/ask", middleware.JWT(a, handlers.AskRAG(a)))
	r.Post("/ask", middleware.JWT(a, handlers.AskRAG(a)))
}

func registerSearchRoutes(r chi.Router, a *app.App) {
	r.Get("/search", middleware.JWT(a, handlers.SearchPosts(a)))
}

func registerRecommendationRoutes(r chi.Router, a *app.App) {
	r.Get("/recommendations", middleware.JWT(a, handlers.GetRecommendations(a)))
}
