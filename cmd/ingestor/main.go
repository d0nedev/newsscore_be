package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/d0nedev/newsscore/internal/ingest"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/logging"
)

// Run exactly one ingestor: two instances would scrape twice and double the block risk.
func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		return 1
	}

	logger := logging.New(logging.Config{
		ServiceName: cfg.App.ServiceName + "-ingestor",
		Environment: cfg.App.Env,
		Level:       cfg.App.LogLevel,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		logger.Error("failed to connect to database", slog.Any("error", err))
		return 1
	}
	defer pool.Close()

	worker := ingest.NewWorker(db.New(pool), logger, cfg.Ingest.DetailsPerRun)
	logger.Info("ingestor started", slog.Duration("interval", cfg.Ingest.Interval))

	ticker := time.NewTicker(cfg.Ingest.Interval)
	defer ticker.Stop()

	for {
		started := time.Now()
		if err := worker.Sync(ctx); err != nil && ctx.Err() == nil {
			logger.Error("sync failed", slog.Any("error", err))
		} else {
			logger.Info("sync done", slog.Duration("took", time.Since(started)))
		}

		select {
		case <-ctx.Done():
			logger.Info("ingestor stopped")
			return 0
		case <-ticker.C:
		}
	}
}
