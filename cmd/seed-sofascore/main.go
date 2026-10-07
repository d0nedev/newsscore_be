// Command seed-sofascore fills the Indonesian competitions Flashscore lacks
// (Liga Nusantara, EPA, Liga Topskor) from SofaScore's API in one go: matches,
// tables, top players, squads, and every finished match's details. The
// ingestor keeps them current afterwards; this only skips its per-run caps.
//
//	go run ./cmd/seed-sofascore
//
// When SofaScore answers 403 "challenge", run scrap-claude/sofascore_proxy.py
// and set SOFASCORE_PROXY=http://localhost:8787.
//
// -images skips the API and only downloads missing team logos and player
// photos (img.sofascore.com has no captcha):
//
//	go run ./cmd/seed-sofascore -images
//
// A new competition is one row: INSERT INTO competitions (slug, name,
// country_id, type, sofascore_id, scraped) VALUES (..., false).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/d0nedev/newsscore/internal/ingest"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/sofascore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	images := flag.Bool("images", false, "only download missing team logos and player photos")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sofascore.SetProxy(cfg.Ingest.SofascoreProxy)
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	w := ingest.NewWorker(db.New(pool), slog.Default(), 0, cfg.App.AssetsDir)
	if *images {
		return w.MirrorAllImages(ctx)
	}
	return w.BackfillSofascore(ctx)
}
