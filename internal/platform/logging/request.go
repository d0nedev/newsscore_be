package logging

import (
	"context"
	"github.com/d0nedev/newsscore/internal/platform/requestcontext"
	"log/slog"
	"net"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

func RequestAttrs(r *http.Request) []any {
	attrs := []any{
		slog.String(
			"request_id",
			requestcontext.GetRequestID(r.Context()),
		),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("user_agent", r.UserAgent()),
		slog.String("remote_ip", clientIP(r)),
	}

	attrs = append(
		attrs,
		TraceAttrs(r.Context())...,
	)

	return attrs
}

func clientIP(r *http.Request) string {
	if ip := chimiddleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func TraceAttrs(ctx context.Context) []any {
	spanContext := trace.SpanFromContext(ctx).SpanContext()

	if !spanContext.IsValid() {
		return nil
	}

	return []any{
		slog.String("trace_id", spanContext.TraceID().String()),
		slog.String("span_id", spanContext.SpanID().String()),
	}
}
