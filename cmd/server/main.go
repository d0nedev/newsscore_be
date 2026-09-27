package main

import (
	"context"
	"errors"
	"github.com/d0nedev/newsscore/internal/app"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	application, err := app.New(context.Background(), version)
	if err != nil {
		slog.Error(
			"failed to initialize application",
			slog.Any("error", err),
		)
		return 1
	}

	cfg := application.Config.App

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           application.Handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	serverErrCh := make(chan error, 1)

	go func() {
		application.Logger.Info(
			"server started",
			slog.String("addr", server.Addr),
			slog.String("version", version),
		)
		serverErrCh <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	exitCode := 0

	select {
	case sig := <-shutdownSignal:
		application.Logger.Info(
			"shutdown signal received",
			slog.String("signal", sig.String()),
		)

		// Fail readiness first so the load balancer stops sending traffic, then drain.
		application.Health.StartDraining()

		if cfg.ShutdownDrainDelay > 0 {
			application.Logger.Info(
				"draining before shutdown",
				slog.Duration("delay", cfg.ShutdownDrainDelay),
			)
			time.Sleep(cfg.ShutdownDrainDelay)
		}

	case err := <-serverErrCh:
		if !errors.Is(err, http.ErrServerClosed) {
			application.Logger.Error(
				"server failed",
				slog.Any("error", err),
			)
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		application.Logger.Error("server shutdown error", slog.Any("error", err))
		exitCode = 1
	}

	// Telemetry flush is best-effort: an unreachable collector must not turn a
	// clean stop into a crash signal for the orchestrator.
	if err := application.Shutdown(shutdownCtx); err != nil {
		application.Logger.Error("application shutdown error", slog.Any("error", err))
	}

	application.Logger.Info("server stopped")

	return exitCode
}
