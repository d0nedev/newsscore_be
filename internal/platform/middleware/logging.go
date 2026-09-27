package middleware

import (
	"github.com/d0nedev/newsscore/internal/platform/logging"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}

	w.status = status
	w.wroteHeader = true

	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	n, err := w.ResponseWriter.Write(body)
	w.bytes += n

	return n, err
}

// Unwrap lets http.ResponseController reach Flush/Hijack/deadlines on the original writer.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// maxLoggedBody caps how much of the request body is kept for the access log.
const maxLoggedBody = 2048

// bodyRecorder keeps the first maxLoggedBody bytes the handler reads, so the
// body can be logged without buffering it up front.
type bodyRecorder struct {
	io.ReadCloser
	buf []byte
}

func (b *bodyRecorder) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if room := maxLoggedBody - len(b.buf); room > 0 {
		b.buf = append(b.buf, p[:min(n, room)]...)
	}

	return n, err
}

func (w *responseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}

	return w.status
}

// Logging writes one access log per request. Requests to quietPaths (health
// probes) are only logged when they fail, and then at WARN, so probe traffic
// doesn't drown real requests or trigger error alerts during a drain.
func Logging(logger *slog.Logger, quietPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseWriter{ResponseWriter: w}

			body := &bodyRecorder{ReadCloser: r.Body}
			r.Body = body

			next.ServeHTTP(rw, r)

			status := rw.Status()
			quiet := slices.Contains(quietPaths, r.URL.Path)

			if quiet && status < http.StatusBadRequest {
				return
			}

			attrs := append(logging.RequestAttrs(r),
				slog.Int("status", status),
				slog.Int("response_bytes", rw.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)

			level := slog.LevelInfo
			switch {
			case quiet:
				level = slog.LevelWarn
			case status >= http.StatusInternalServerError:
				level = slog.LevelError
			case status >= http.StatusBadRequest:
				level = slog.LevelWarn
			}

			// Body only on failures: keeps volume and PII out of normal traffic logs.
			if level > slog.LevelInfo && len(body.buf) > 0 {
				attrs = append(attrs, slog.String("request_body", string(body.buf)))
			}

			logger.Log(r.Context(), level, "http request completed", attrs...)
		})
	}
}
