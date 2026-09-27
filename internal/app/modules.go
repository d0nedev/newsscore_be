package app

import (
	"log/slog"

	"github.com/d0nedev/newsscore/internal/platform/config"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

// modules wires every domain and returns the routes each one mounts under /api/v1.
// To add a domain: build its service/handler here and append its RegisterRoutes.
func modules(
	cfg *config.Config,
	logger *slog.Logger,
	pool *pgxpool.Pool,
	tp trace.TracerProvider,
) []func(chi.Router) {
	return nil
}
