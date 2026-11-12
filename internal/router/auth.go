package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
)

func registerAuthRoutes(r chi.Router, a *app.App) {
	r.Post("/v1/login", handlers.Login(a))
	r.Post("/login", handlers.Login(a))
	r.Post("/refresh", handlers.Refresh(a))
	r.Post("/logout", middleware.JWT(a, handlers.Logout(a)))
}
