package tracing

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/d0nedev/newsscore/internal/platform/apperror"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecordError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus codes.Code
		wantEvents int
	}{
		{"nil", nil, codes.Unset, 0},
		{"plain error", errors.New("boom"), codes.Error, 1},
		{"client apperror", apperror.New(http.StatusNotFound, "NOT_FOUND", "nf"), codes.Unset, 0},
		{"server apperror", apperror.New(http.StatusInternalServerError, "DB", "db"), codes.Error, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

			_, span := tp.Tracer("test").Start(context.Background(), "op")
			RecordError(span, tt.err)
			span.End()

			got := sr.Ended()[0]
			if got.Status().Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", got.Status().Code, tt.wantStatus)
			}
			if len(got.Events()) != tt.wantEvents {
				t.Errorf("events = %d, want %d", len(got.Events()), tt.wantEvents)
			}
		})
	}
}
