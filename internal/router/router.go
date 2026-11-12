package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	_ "github.com/shreyasreddy45/RSS-server/docs"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
	"github.com/shreyasreddy45/RSS-server/internal/web"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// New builds the full chi router, wiring up global middleware and mounting
// the v1 API routes. This mirrors the old setupRouter function in main.go.
func New(a *app.App) *chi.Mux {

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.RateLimit(a.Logger))
	r.Use(middleware.Recovery(a.Logger))
	r.Use(middleware.RequestID)
	r.Use(middleware.SecurityHeaders)

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	v1Router := chi.NewRouter()

	registerHealthRoutes(v1Router, a)
	registerUserRoutes(v1Router, a)
	registerAuthRoutes(v1Router, a)
	registerFeedRoutes(v1Router, a)
	registerPostRoutes(v1Router, a)
	registerSearchRoutes(v1Router, a)
	registerRecommendationRoutes(v1Router, a)
	registerAgentRoutes(v1Router, a)

	r.Mount("/v1", v1Router)

	// The web frontend is served from the same origin as the API.
	r.Handle("/*", web.Handler())

	return r
}
