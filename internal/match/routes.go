package match

import (
	"log/slog"
	"net/http"

	"github.com/d0nedev/newsscore/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(r chi.Router, h *Handler, hub http.Handler, logger *slog.Logger) {
	r.Route("/matches", func(r chi.Router) {
		r.Get("/", httpx.Handle(logger, h.List))
		r.Get("/stream", hub.ServeHTTP)
		r.Get("/{id}", httpx.Handle(logger, h.FindByID))
	})
}
