// Package search finds teams, players, and published news by name, typo-tolerant (pg_trgm).
package search

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

const CodeSearchFailed = "SEARCH_FAILED"

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

type results struct {
	Teams   []db.SearchTeamsRow
	Players []db.SearchPlayersRow
	News    []db.SearchNewsRow
}

func (s *Service) Search(ctx context.Context, q string) (results, error) {
	ctx, span := s.tracer.Start(ctx, "SearchService.Search")
	defer span.End()

	fail := func(err error) (results, error) {
		return results{}, tracing.Fail(span, apperror.Internal(CodeSearchFailed, "search failed", err))
	}

	var r results
	var err error
	if r.Teams, err = s.queries.SearchTeams(ctx, q); err != nil {
		return fail(err)
	}
	if r.Players, err = s.queries.SearchPlayers(ctx, q); err != nil {
		return fail(err)
	}
	if r.News, err = s.queries.SearchNews(ctx, q); err != nil {
		return fail(err)
	}
	return r, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger) {
	r.Get("/search", httpx.Handle(logger, h.Search))
}

type Team struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Badge string `json:"badge"`
	Logo  string `json:"logo,omitempty"`
}

type Player struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number,omitempty"`
	Team   string `json:"team,omitempty"`
}

type News struct {
	ID          string `json:"id"` // slug
	Title       string `json:"title"`
	Category    string `json:"category"`
	PublishedAt string `json:"publishedAt"`
}

type Response struct {
	Data struct {
		Teams   []Team   `json:"teams"`
		Players []Player `json:"players"`
		News    []News   `json:"news"`
	} `json:"data"`
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) error {
	q := strings.Join(strings.Fields(r.URL.Query().Get("q")), " ")
	if n := utf8.RuneCountInString(q); n < 2 || n > 64 {
		return apperror.Validation("q must be 2 to 64 characters")
	}

	res, err := h.service.Search(r.Context(), q)
	if err != nil {
		return err
	}

	var out Response
	out.Data.Teams = make([]Team, 0, len(res.Teams))
	out.Data.Players = make([]Player, 0, len(res.Players))
	out.Data.News = make([]News, 0, len(res.News))
	for _, t := range res.Teams {
		out.Data.Teams = append(out.Data.Teams, Team{ID: t.ID.String(), Name: t.Name, Badge: t.ShortName, Logo: t.LogoUrl.String})
	}
	for _, p := range res.Players {
		out.Data.Players = append(out.Data.Players, Player{ID: p.ID.String(), Name: p.Name, Number: int(p.ShirtNumber.Int16), Team: p.TeamName.String})
	}
	for _, n := range res.News {
		out.Data.News = append(out.Data.News, News{ID: n.Slug, Title: n.Title, Category: n.Category, PublishedAt: n.PublishedAt.Time.UTC().Format(time.RFC3339)})
	}

	return httpx.WriteJSON(w, http.StatusOK, out)
}
