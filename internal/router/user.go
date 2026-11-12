package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
)

func registerUserRoutes(r chi.Router, a *app.App) {
	r.Post("/users", handlers.CreateUser(a))
	r.Get("/users", middleware.JWT(a, handlers.GetUsers(a)))
}
