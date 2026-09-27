package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	"github.com/d0nedev/newsscore/internal/platform/health"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/logging"
	"github.com/d0nedev/newsscore/internal/platform/metrics"
	"github.com/d0nedev/newsscore/internal/platform/middleware"
	"github.com/d0nedev/newsscore/internal/platform/stream"
	"github.com/d0nedev/newsscore/internal/platform/tracing"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/sdk/trace"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	livenessPath  = "/health"
	readinessPath = "/ready"
)

type App struct {
	Handler        http.Handler
	Pool           *pgxpool.Pool
	Config         *config.Config
	Logger         *slog.Logger
	TraceProvider  *trace.TracerProvider
	MetricProvider *sdkmetric.MeterProvider
	Health         *health.Handler
	// Hub holds the open SSE streams; close it when shutdown starts so they do not block draining.
	Hub *stream.Hub

	stopBackground context.CancelFunc
}

func New(ctx context.Context, version string) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := logging.New(logging.Config{
		ServiceName: cfg.App.ServiceName,
		Environment: cfg.App.Env,
		Level:       cfg.App.LogLevel,
	})

	tp, err := tracing.Init(tracing.Config{
		ServiceName: cfg.App.ServiceName,
		Version:     version,
		Environment: cfg.App.Env,
		Endpoint:    cfg.OTLP.Endpoint,
		Insecure:    cfg.OTLP.Insecure,
		SampleRate:  cfg.Tracing.SampleRate,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize tracing: %w", err)
	}

	meterProvider, err := metrics.Init(metrics.Config{
		ServiceName: cfg.App.ServiceName,
		Version:     version,
		Environment: cfg.App.Env,
		Endpoint:    cfg.OTLP.Endpoint,
		Insecure:    cfg.OTLP.Insecure,
	})
	if err != nil {
		_ = tracing.Shutdown(context.Background(), tp)

		return nil, fmt.Errorf("initialize metrics: %w", err)
	}

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		_ = metrics.Shutdown(context.Background(), meterProvider)
		_ = tracing.Shutdown(context.Background(), tp)

		return nil, fmt.Errorf("initialize database: %w", err)
	}

	logger.Info("configuration loaded",
		slog.String("version", version),
		slog.Float64("trace_sample_rate", cfg.Tracing.SampleRate),
		slog.Bool("otlp_insecure", cfg.OTLP.Insecure),
		slog.Int("rate_limit_rpm", cfg.RateLimit.RequestsPerMinute),
		slog.Any("trusted_proxies", cfg.App.TrustedProxies),
	)

	healthHandler := health.NewHandler(pool)

	// Background work (LISTEN for live updates) lives until Shutdown, not until ctx.
	bgCtx, stopBackground := context.WithCancel(context.Background())
	hub := stream.NewHub(logger)

	router := newRouter(cfg, logger, healthHandler, modules(bgCtx, cfg, logger, pool, tp, hub)...)

	handler := otelhttp.NewHandler(router, cfg.App.ServiceName,
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != livenessPath && r.URL.Path != readinessPath
		}),
	)

	return &App{
		Handler:        handler,
		Pool:           pool,
		Config:         cfg,
		Logger:         logger,
		TraceProvider:  tp,
		MetricProvider: meterProvider,
		Health:         healthHandler,
		Hub:            hub,
		stopBackground: stopBackground,
	}, nil
}

func (a *App) Shutdown(ctx context.Context) error {
	var errs []error

	if a.stopBackground != nil {
		a.stopBackground()
	}
	if a.Hub != nil {
		a.Hub.Close()
	}

	if a.MetricProvider != nil {
		if err := metrics.Shutdown(ctx, a.MetricProvider); err != nil {
			errs = append(errs, fmt.Errorf("shutdown metrics: %w", err))
		}
	}

	if a.TraceProvider != nil {
		if err := tracing.Shutdown(ctx, a.TraceProvider); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracing: %w", err))
		}
	}

	if a.Pool != nil {
		a.Pool.Close()
	}

	return errors.Join(errs...)
}

// newRouter builds the HTTP surface shared by every service; domain routes are
// mounted under /api/v1 from the registrars returned by modules().
func newRouter(
	cfg *config.Config,
	logger *slog.Logger,
	healthHandler *health.Handler,
	apiRoutes ...func(chi.Router),
) *chi.Mux {
	router := chi.NewRouter()

	// Logging wraps Recovery so recovered panics still produce an access log with status 500.
	router.Use(middleware.ClientIP(cfg.App.TrustedProxies))
	router.Use(middleware.RequestID)
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.CORS(cfg.Auth.AllowedOrigins))
	router.Use(middleware.Logging(logger, livenessPath, readinessPath))
	router.Use(middleware.RouteTag)
	router.Use(middleware.Recovery(logger))

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, apperror.New(http.StatusNotFound, apperror.CodeNotFound, "route not found"))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, apperror.New(http.StatusMethodNotAllowed, apperror.CodeMethodNotAllowed, "method not allowed"))
	})

	router.Get(livenessPath, healthHandler.Health)
	router.Get(readinessPath, healthHandler.Ready)

	router.Route("/api/v1", func(r chi.Router) {
		// End the request context before WriteTimeout so the error response still reaches the client.
		r.Use(middleware.RequestTimeout(cfg.App.WriteTimeout - min(time.Second, cfg.App.WriteTimeout/10)))
		r.Use(middleware.RateLimit(cfg.RateLimit.RequestsPerMinute))

		for _, mount := range apiRoutes {
			mount(r)
		}
	})

	return router
}
