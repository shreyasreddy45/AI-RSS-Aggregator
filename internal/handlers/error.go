package handlers

import (
	"net/http"

	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

func Err(w http.ResponseWriter, r *http.Request) {
	jsonutil.RespondWithError(w, 400, "Bad request")
}
