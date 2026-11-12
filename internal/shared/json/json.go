package json

import (
	"encoding/json"
	"log"
	"net/http"
)

func RespondWithError(
	w http.ResponseWriter,
	code int,
	message string,
) {

	if code >= 500 {
		log.Printf(
			"HTTP %d: %s",
			code,
			message,
		)
	}

	ResponseWithJSON(
		w,
		code,
		map[string]string{
			"error": message,
		},
	)
}
func ResponseWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", payload)
		w.WriteHeader(500)
		return

	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}
