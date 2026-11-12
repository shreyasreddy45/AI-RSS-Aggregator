package handlers

import (
	"net/http"

	jsonutil "github.com/shreyasreddy45/RSS-server/internal/shared/json"
)

func Readlines(w http.ResponseWriter, r *http.Request) {
	jsonutil.ResponseWithJSON(w, 200, struct{}{})
}
