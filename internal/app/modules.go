package app

import (
	"context"
	"log/slog"

	"github.com/d0nedev/newsscore/internal/auth"
	"github.com/d0nedev/newsscore/internal/match"
	"github.com/d0nedev/newsscore/internal/news"
	"github.com/d0nedev/newsscore/internal/platform/config"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/middleware"
	"github.com/d0nedev/newsscore/internal/platform/stream"
	"github.com/d0nedev/newsscore/internal/player"
	"github.com/d0nedev/newsscore/internal/search"
	"github.com/d0nedev/newsscore/internal/standing"
	"github.com/d0nedev/newsscore/internal/team"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

// modules wires every domain and returns the routes each one mounts under /api/v1.
// To add a domain: build its service/handler here and append its RegisterRoutes.
// Background goroutines started here must stop when ctx is canceled.
func modules(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	pool *pgxpool.Pool,
	tp trace.TracerProvider,
	hub *stream.Hub,
) []func(chi.Router) {
	queries := db.New(pool)

	authHandler := auth.NewHandler(
		auth.NewService(queries, tp.Tracer("auth"), cfg.Auth.SessionTTL),
		cfg.Auth.CookieSecure,
	)
	// ponytail: 10 login attempts per IP per minute, in memory per replica; move to a shared store if replicas multiply.
	loginLimit := middleware.RateLimit(10)

	newsHandler := news.NewHandler(news.NewService(pool, queries, tp.Tracer("news")))

	standings := standing.NewHandler(standing.NewService(queries, tp.Tracer("standing")))

	teams := team.NewHandler(team.NewService(queries, tp.Tracer("team")))
	players := player.NewHandler(player.NewService(queries, tp.Tracer("player")))

	searcher := search.NewHandler(search.NewService(queries, tp.Tracer("search")))

	matches := match.NewHandler(match.NewService(queries, tp.Tracer("match")))
	go stream.Listen(ctx, pool, logger, match.NotifyChannel, match.Relay(hub, logger))

	return []func(chi.Router){
		func(r chi.Router) { match.RegisterRoutes(r, matches, hub, logger) },
		func(r chi.Router) { auth.RegisterRoutes(r, authHandler, logger, loginLimit) },
		func(r chi.Router) { standing.RegisterRoutes(r, standings, logger) },
		func(r chi.Router) { team.RegisterRoutes(r, teams, logger) },
		func(r chi.Router) { player.RegisterRoutes(r, players, logger) },
		func(r chi.Router) { search.RegisterRoutes(r, searcher, logger) },
		func(r chi.Router) { news.RegisterRoutes(r, newsHandler, logger, authHandler.RequireAdmin) },
	}
}
