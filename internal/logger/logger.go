package logger

import (
	"log/slog"
	"os"
)

// New creates the application's structured logger.
// This mirrors the global `logger` slog.Logger that used to live in main.go.
func New() *slog.Logger {
	return slog.New(
		slog.NewTextHandler(os.Stdout, nil),
	)
}
