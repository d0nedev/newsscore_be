package match

import (
	"net/http"
	"uuid"

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
	filter, err := parseListFilter(r.URL.Query())
	if err != nil {
		return err
	}

	matches, err := h.service.List(r.Context(), filter)
	if err != nil {
		return err
	}

	response := ListMatchesResponse{Data: make([]MatchResponse, 0, len(matches))}
	for _, m := range matches {
		response.Data = append(response.Data, toMatchResponse(m))
	}

	return httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) FindByID(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return apperror.Validation("invalid match id")
	}

	detail, err := h.service.FindByID(r.Context(), id)
	if err != nil {
		return err
	}

	return httpx.WriteJSON(w, http.StatusOK, DataResponse{Data: toMatchDetailResponse(detail)})
}
