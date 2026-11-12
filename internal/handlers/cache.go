package handlers

import (
	"net/http"

	"github.com/shreyasreddy45/RSS-server/internal/app"
	cacheutil "github.com/shreyasreddy45/RSS-server/internal/redis"
)

// invalidateContentCache drops cached feed and post responses. It is called
// whenever a feed or a follow changes so the UI never shows a stale list.
func invalidateContentCache(a *app.App, r *http.Request) {
	if a.Redis == nil {
		return
	}
	if err := cacheutil.InvalidateFeedAndPostCache(r.Context(), a.Redis); err != nil {
		a.Logger.Error("Failed to invalidate response cache", "error", err)
	}
}
