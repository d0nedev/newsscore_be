package logging

import (
	"errors"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"log/slog"
	"net/http"
)

func LogError(
	logger *slog.Logger,
	r *http.Request,
	err error,
) {
	attrs := RequestAttrs(r)

	var appErr *apperror.Error

	if errors.As(err, &appErr) {
		attrs = append(attrs,
			slog.String("error_code", appErr.Code),
			slog.Int("status", appErr.Status),
			slog.Any("cause", appErr.Err),
		)

		if appErr.Status < http.StatusInternalServerError {
			logger.WarnContext(r.Context(), appErr.Message, attrs...)
			return
		}

		attrs = append(attrs, slog.Any("stack_trace", appErr.StackTrace()))
		logger.ErrorContext(r.Context(), appErr.Message, attrs...)

		return
	}

	attrs = append(attrs,
		slog.Any("error", err),
	)

	logger.ErrorContext(
		r.Context(),
		"unexpected error",
		attrs...,
	)
}
