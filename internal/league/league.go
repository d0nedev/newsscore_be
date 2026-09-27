// Package league serves competitions and their tables, computed from finished matches.
package league

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

const (
	CodeLeagueNotFound    = "LEAGUE_NOT_FOUND"
	CodeLeagueQueryFailed = "LEAGUE_QUERY_FAILED"
)

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

func (s *Service) List(ctx context.Context) ([]db.ListCompetitionsRow, error) {
	ctx, span := s.tracer.Start(ctx, "LeagueService.List")
	defer span.End()

	rows, err := s.queries.ListCompetitions(ctx)
	if err != nil {
		return nil, tracing.Fail(span, apperror.Internal(CodeLeagueQueryFailed, "failed to list leagues", err))
	}
	return rows, nil
}

// Get returns a competition and its table for season; season 0 means its latest season.
func (s *Service) Get(ctx context.Context, slug string, season int16) (db.GetCompetitionRow, []db.ListStandingsRow, error) {
	ctx, span := s.tracer.Start(ctx, "LeagueService.Get")
	defer span.End()

	c, err := s.queries.GetCompetition(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			return c, nil, apperror.NotFound(CodeLeagueNotFound, "league not found")
		}
		return c, nil, tracing.Fail(span, apperror.Internal(CodeLeagueQueryFailed, "failed to get league", err))
	}
	if season != 0 {
		c.Season = season
	}

	rows, err := s.queries.ListStandings(ctx, db.ListStandingsParams{CompetitionID: c.ID, Season: c.Season})
	if err != nil {
		return c, nil, tracing.Fail(span, apperror.Internal(CodeLeagueQueryFailed, "failed to compute standings", err))
	}
	return c, rows, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger) {
	r.Get("/leagues", httpx.Handle(logger, h.List))
	r.Get("/leagues/{slug}", httpx.Handle(logger, h.Get))
}

// League follows the contract: id is the slug. Standings is empty for cups.
type League struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	Type      string `json:"type"`
	Season    int    `json:"season"`
	Standings *[]Row `json:"standings,omitempty"` // detail only; a pointer so cups still send []
}

// Row follows the contract's StandingRow; form is newest first.
type Row struct {
	Position     int      `json:"position"`
	TeamID       string   `json:"teamId"`
	Team         string   `json:"team"`
	Badge        string   `json:"badge"`
	Logo         string   `json:"logo,omitempty"`
	Played       int32    `json:"played"`
	Won          int32    `json:"won"`
	Drawn        int32    `json:"drawn"`
	Lost         int32    `json:"lost"`
	GoalsFor     int32    `json:"goalsFor"`
	GoalsAgainst int32    `json:"goalsAgainst"`
	Points       int32    `json:"points"`
	Form         []string `json:"form"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.service.List(r.Context())
	if err != nil {
		return err
	}
	out := make([]League, 0, len(rows))
	for _, c := range rows {
		out = append(out, League{ID: c.Slug, Name: c.Name, Country: c.Country, Type: c.Type, Season: int(c.Season)})
	}
	return httpx.WriteJSON(w, http.StatusOK, struct {
		Data []League `json:"data"`
	}{out})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	var season int16
	if v := r.URL.Query().Get("season"); v != "" {
		n, err := strconv.ParseInt(v, 10, 16)
		if err != nil || n < 1900 {
			return apperror.Validation("season must be a year, e.g. 2026")
		}
		season = int16(n)
	}

	c, rows, err := h.service.Get(r.Context(), chi.URLParam(r, "slug"), season)
	if err != nil {
		return err
	}

	standings := []Row{}
	out := League{ID: c.Slug, Name: c.Name, Country: c.Country, Type: c.Type, Season: int(c.Season), Standings: &standings}
	// ponytail: cups get no table; group-stage tables need a group column on matches first.
	if c.Type == "league" {
		for i, t := range rows {
			standings = append(standings, Row{
				Position: i + 1, TeamID: t.ID.String(), Team: t.Name, Badge: t.ShortName, Logo: t.LogoUrl.String,
				Played: t.Played, Won: t.Won, Drawn: t.Drawn, Lost: t.Lost,
				GoalsFor: t.GoalsFor, GoalsAgainst: t.GoalsAgainst, Points: t.Points, Form: t.Form,
			})
		}
	}

	return httpx.WriteJSON(w, http.StatusOK, struct {
		Data League `json:"data"`
	}{out})
}
