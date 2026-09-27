// Package stream fans Server-Sent Events out to every connected client.
package stream

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	heartbeatInterval = 20 * time.Second // keeps proxies from closing idle streams
	clientBuffer      = 32
	maxStreamAge      = 30 * time.Minute // bounds a connection; EventSource reconnects on its own
)

// Hub broadcasts pre-rendered SSE frames. A client that cannot keep up is
// disconnected rather than allowed to block the broadcast; it reconnects and refetches.
type Hub struct {
	logger *slog.Logger

	mu      sync.Mutex
	clients map[chan []byte]struct{}
	closed  bool
	done    chan struct{}
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{logger: logger, clients: make(map[chan []byte]struct{}), done: make(chan struct{})}
}

// Publish sends one event to every client.
func (h *Hub) Publish(event string, data []byte) {
	frame := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, data)

	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		select {
		case c <- frame:
		default:
			delete(h.clients, c)
			close(c)
		}
	}
}

// Close ends every stream so http.Server.Shutdown does not wait on them.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.closed {
		h.closed = true
		close(h.done)
	}
}

func (h *Hub) subscribe() (chan []byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil, false
	}
	c := make(chan []byte, clientBuffer)
	h.clients[c] = struct{}{}
	return c, true
}

func (h *Hub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c)
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	// The server's WriteTimeout would cut the stream; each write gets its own deadline instead.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && err != http.ErrNotSupported {
		h.logger.Warn("stream: clear write deadline", slog.Any("error", err))
	}

	c, ok := h.subscribe()
	if !ok {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	defer h.unsubscribe(c)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(b []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := w.Write(b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	if !write([]byte("retry: 3000\n\n")) {
		return
	}

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	maxAge := time.NewTimer(maxStreamAge)
	defer maxAge.Stop()

	for {
		select {
		case frame, open := <-c:
			if !open || !write(frame) {
				return
			}
		case <-heartbeat.C:
			if !write([]byte(": ping\n\n")) {
				return
			}
		case <-r.Context().Done():
			return
		case <-h.done:
			return
		case <-maxAge.C:
			return
		}
	}
}

// Listen relays Postgres NOTIFY payloads on channel to fn until ctx ends,
// reconnecting with backoff when the connection drops.
func Listen(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, channel string, fn func(payload string)) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := listenOnce(ctx, pool, channel, fn, func() { backoff = time.Second })
		if ctx.Err() != nil {
			return
		}
		logger.Error("stream: listen failed, retrying", slog.String("channel", channel), slog.Any("error", err), slog.Duration("backoff", backoff))

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, channel string, fn func(string), connected func()) error {
	pooled, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// LISTEN state is per-connection; take it out of the pool for good.
	conn := pooled.Hijack()
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return err
	}
	connected()

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		fn(n.Payload)
	}
}
