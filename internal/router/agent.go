package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/shreyasreddy45/RSS-server/internal/app"
	"github.com/shreyasreddy45/RSS-server/internal/handlers"
	"github.com/shreyasreddy45/RSS-server/internal/middleware"
)

func registerAgentRoutes(r chi.Router, a *app.App) {
	r.Post("/agent/chat", middleware.JWT(a, handlers.AgentChat(a)))
}
