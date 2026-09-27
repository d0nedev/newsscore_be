package news

import (
	"log/slog"
	"net/http"

	"github.com/d0nedev/newsscore/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts public /news and editor /admin/news (behind requireAdmin).
func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger, requireAdmin func(http.Handler) http.Handler) {
	r.Get("/news", httpx.Handle(logger, h.List))
	r.Get("/news/{slug}", httpx.Handle(logger, h.Get))

	r.With(requireAdmin).Route("/admin/news", func(r chi.Router) {
		r.Get("/", httpx.Handle(logger, h.AdminList))
		r.Post("/", httpx.Handle(logger, h.AdminCreate))
		r.Get("/{id}", httpx.Handle(logger, h.AdminGet))
		r.Put("/{id}", httpx.Handle(logger, h.AdminUpdate))
		r.Delete("/{id}", httpx.Handle(logger, h.AdminDelete))
	})
}
