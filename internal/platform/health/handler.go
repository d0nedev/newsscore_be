package health

import (
	"context"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool     *pgxpool.Pool
	draining atomic.Bool
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{
		pool: pool,
	}
}

// StartDraining makes /ready fail so load balancers stop routing new traffic
// before the server shuts down.
func (h *Handler) StartDraining() {
	h.draining.Store(true)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.draining.Load() {
		_ = httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "shutting_down",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		_ = httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
		})
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}
