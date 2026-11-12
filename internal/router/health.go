package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
)

func registerHealthRoutes(r chi.Router, a *app.App) {
	r.Get("/health/live", handlers.Live)
	r.Get("/health/ready", handlers.Ready(a))
	r.Get("/error", handlers.Err)
}
