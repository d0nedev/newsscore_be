package middleware

import (
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/logging"
	"github.com/d0nedev/newsscore/internal/platform/tracing"
	"log/slog"
	"net/http"
	"runtime/debug"

	"go.opentelemetry.io/otel/trace"
)

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w}

			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				// net/http uses ErrAbortHandler to abort a response silently; keep that contract.
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				tracing.RecordPanic(trace.SpanFromContext(r.Context()), recovered)

				attrs := append(logging.RequestAttrs(r),
					slog.Int("status", http.StatusInternalServerError),
					slog.Any("panic", recovered),
					slog.Bool("headers_already_sent", rw.wroteHeader),
					slog.String("stack_trace", string(debug.Stack())),
				)

				logger.ErrorContext(r.Context(), "panic recovered", attrs...)

				// Status and part of the body are already on the wire; a second
				// write would only corrupt the response.
				if rw.wroteHeader {
					return
				}

				httpx.WriteInternalServerError(rw)
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
