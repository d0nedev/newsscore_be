package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/d0nedev/newsscore/internal/ingest"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/logging"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"
	"github.com/d0nedev/newsscore/internal/provider/sofascore"
)

// Run exactly one ingestor: two instances would scrape twice and double the block risk.
func main() {
	os.Exit(run())
}

func run() int {
	backfill := flag.Bool("backfill-lineups", false, "read lineups again for matches stored before formations, slots, and ratings were kept, then exit")
	flag.Parse()

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

	flashscore.SetFSign(cfg.Ingest.FSign)
	sofascore.SetProxy(cfg.Ingest.SofascoreProxy)
	worker := ingest.NewWorker(db.New(pool), logger, cfg.Ingest.DetailsPerRun, cfg.App.AssetsDir)
	if *backfill {
		if err := worker.BackfillLineups(ctx); err != nil {
			logger.Error("backfill lineups failed", slog.Any("error", err))
			return 1
		}
		return 0
	}
	logger.Info("ingestor started",
		slog.Duration("interval", cfg.Ingest.Interval),
		slog.Duration("live_interval", cfg.Ingest.LiveInterval),
	)

	// Separate loops: a slow league sync (details backfill) must not delay live scores.
	var wg sync.WaitGroup
	wg.Go(func() { every(ctx, logger, "league", cfg.Ingest.Interval, worker.Sync) })
	wg.Go(func() { every(ctx, logger, "live", cfg.Ingest.LiveInterval, worker.SyncLive) })
	wg.Wait()

	logger.Info("ingestor stopped")
	return 0
}

// every runs fn now and then on each tick until ctx is done.
func every(ctx context.Context, logger *slog.Logger, name string, interval time.Duration, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		started := time.Now()
		if err := fn(ctx); err != nil && ctx.Err() == nil {
			logger.Error("sync failed", slog.String("loop", name), slog.Any("error", err))
		} else {
			logger.Debug("sync done", slog.String("loop", name), slog.Duration("took", time.Since(started)))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
