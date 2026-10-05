// Command seed-sofascore fills the Indonesian competitions Flashscore lacks
// (Liga Nusantara, EPA, Liga Topskor) from SofaScore's API in one go: matches,
// tables, top players, squads, and every finished match's details. The
// ingestor keeps them current afterwards; this only skips its per-run caps.
//
//	go run ./cmd/seed-sofascore
//
// A new competition is one row: INSERT INTO competitions (slug, name,
// country_id, type, sofascore_id, scraped) VALUES (..., false).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/d0nedev/newsscore/internal/ingest"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	return ingest.NewWorker(db.New(pool), slog.Default(), 0, cfg.App.AssetsDir).BackfillSofascore(ctx)
}
