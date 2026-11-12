package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/shreyasreddy45/RSS-server/internal/app"
	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

// Live reports the service liveness status.
//
//	@Summary	Liveness check
//	@Tags		Health
//	@Produce	json
//	@Success	200	{object}	map[string]string
//	@Router		/health/live [get]
func Live(w http.ResponseWriter, r *http.Request) {

	jsonutil.ResponseWithJSON(w, http.StatusOK, map[string]string{
		"status": "alive",
	})
}

// Ready reports the service readiness including database connectivity.
//
//	@Summary	Readiness check
//	@Tags		Health
//	@Produce	json
//	@Success	200	{object}	map[string]string
//	@Failure	503	{object}	map[string]string
//	@Router		/health/ready [get]
func Ready(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		err := a.SQLDB.PingContext(ctx)

		if err != nil {

			jsonutil.RespondWithError(
				w,
				http.StatusServiceUnavailable,
				"Database is not reachable",
			)

			return
		}

		jsonutil.ResponseWithJSON(w, http.StatusOK, map[string]string{
			"status": "ready",
		})
	}
}
