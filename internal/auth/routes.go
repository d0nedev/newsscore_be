package auth

import (
	"log/slog"
	"net/http"

	"github.com/d0nedev/newsscore/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts /auth and /me. loginLimit is a stricter per-IP limiter against password guessing.
func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger, loginLimit func(http.Handler) http.Handler) {
	r.With(loginLimit).Post("/auth/login", httpx.Handle(logger, h.Login))
	r.Post("/auth/logout", httpx.Handle(logger, h.Logout))
	r.With(h.RequireUser).Get("/me", httpx.Handle(logger, h.Me))
}
