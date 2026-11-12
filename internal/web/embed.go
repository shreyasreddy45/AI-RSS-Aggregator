// Package web serves the single-page frontend that ships inside the Go binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// Handler serves the embedded frontend. The app uses hash-based routing, so
// every in-app navigation is served by index.html at "/".
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// Only possible if the embed directive above is wrong; fail loudly at start-up.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
