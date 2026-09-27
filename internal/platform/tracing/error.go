package tracing

import (
	"errors"
	"fmt"
	"github.com/d0nedev/newsscore/internal/platform/apperror"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}

	var appErr *apperror.Error

	if errors.As(err, &appErr) {
		if appErr.Status < 500 {
			return
		}

		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			appErr.Code,
		)

		return
	}

	span.RecordError(err)
	span.SetStatus(
		codes.Error,
		"unexpected error",
	)
}

func RecordPanic(span trace.Span, recoverd any) {
	if recoverd == nil {
		return
	}

	err := fmt.Errorf("panic: %v", recoverd)

	span.RecordError(err)
	span.SetStatus(
		codes.Error,
		"panic recovered",
	)
}

// Fail records err on span (5xx only, see RecordError) and returns it, for one-line error returns.
func Fail(span trace.Span, err error) error {
	RecordError(span, err)
	return err
}
