package news

import (
	"net/http"
	"time"
	"uuid"

	"github.com/d0nedev/newsscore/internal/auth"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	f, err := parseListFilter(r.URL.Query())
	if err != nil {
		return err
	}

	rows, next, err := h.service.List(r.Context(), f)
	if err != nil {
		return err
	}

	now := time.Now()
	resp := ListNewsResponse{Data: make([]NewsItem, 0, len(rows))}
	for _, n := range rows {
		resp.Data = append(resp.Data, toNewsItem(n.Slug, n.Category, n.Title, n.Summary, n.ImageUrl, n.PublishedAt, now))
	}
	if next != nil {
		c := next.encode()
		resp.Meta.NextCursor = &c
	}
	return httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	n, err := h.service.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		return err
	}
	item := toNewsItem(n.Slug, n.Category, n.Title, n.Summary, n.ImageUrl, n.PublishedAt, time.Now())
	return httpx.WriteJSON(w, http.StatusOK, DataResponse{Data: NewsDetail{NewsItem: item, Body: n.Body}})
}

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.service.ListAll(r.Context())
	if err != nil {
		return err
	}
	out := make([]AdminNews, 0, len(rows))
	for _, n := range rows {
		out = append(out, toAdminSummary(n))
	}
	return httpx.WriteJSON(w, http.StatusOK, DataResponse{Data: out})
}

func (h *Handler) AdminGet(w http.ResponseWriter, r *http.Request) error {
	id, err := newsID(r)
	if err != nil {
		return err
	}
	n, err := h.service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, DataResponse{Data: n})
}

func (h *Handler) AdminCreate(w http.ResponseWriter, r *http.Request) error {
	v, err := decodeWrite(w, r)
	if err != nil {
		return err
	}
	user, _ := auth.UserFrom(r.Context())

	id, err := h.service.Create(r.Context(), v, user.ID)
	if err != nil {
		return err
	}
	n, err := h.service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, DataResponse{Data: n})
}

func (h *Handler) AdminUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := newsID(r)
	if err != nil {
		return err
	}
	v, err := decodeWrite(w, r)
	if err != nil {
		return err
	}

	if err := h.service.Update(r.Context(), id, v); err != nil {
		return err
	}
	n, err := h.service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, DataResponse{Data: n})
}

func (h *Handler) AdminDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := newsID(r)
	if err != nil {
		return err
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func newsID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.UUID{}, apperror.Validation("invalid news id")
	}
	return id, nil
}

func decodeWrite(w http.ResponseWriter, r *http.Request) (validated, error) {
	var req WriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return validated{}, err
	}
	return req.validate()
}
