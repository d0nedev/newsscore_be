package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout bounds the request context. http.Server's WriteTimeout only
// closes the connection; it never cancels r.Context(), so without this a
// request stuck on pool acquire or an unresponsive DB would hold its goroutine
// long after the client is gone.
//
// Paths ending in /stream are exempt: SSE connections are meant to stay open
// and bound themselves (see platform/stream).
func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/stream") {
				next.ServeHTTP(w, r)
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
