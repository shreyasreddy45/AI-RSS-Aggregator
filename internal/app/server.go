package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts the HTTP server and blocks until ctx is cancelled (via an
// OS signal or externally), then shuts the server down gracefully.
func (a *App) Run(ctx context.Context, cancel context.CancelFunc, handler http.Handler) {

	srv := &http.Server{
		Addr:    ":" + a.Config.Port,
		Handler: handler,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		a.Logger.Info("Shutdown signal received")
		cancel()
	}()

	go func() {
		a.Logger.Info(
			"Server starting",
			"port", a.Config.Port,
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}

	a.Logger.Info("Server stopped gracefully")
}
