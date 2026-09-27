// Package standing serves the league table, computed from finished matches.
package standing

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

const CodeStandingsQueryFailed = "STANDINGS_QUERY_FAILED"

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

// Table returns the standings for season; season 0 means the latest one with matches.
func (s *Service) Table(ctx context.Context, season int16) (int16, []db.ListStandingsRow, error) {
	ctx, span := s.tracer.Start(ctx, "StandingService.Table")
	defer span.End()

	if season == 0 {
		latest, err := s.queries.LatestSeason(ctx)
		if err != nil {
			return 0, nil, tracing.Fail(span, apperror.Internal(CodeStandingsQueryFailed, "failed to find season", err))
		}
		season = latest
	}

	rows, err := s.queries.ListStandings(ctx, season)
	if err != nil {
		return 0, nil, tracing.Fail(span, apperror.Internal(CodeStandingsQueryFailed, "failed to compute standings", err))
	}
	return season, rows, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger) {
	r.Get("/standings", httpx.Handle(logger, h.Table))
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

type TableResponse struct {
	Data struct {
		Season int   `json:"season"`
		Rows   []Row `json:"rows"`
	} `json:"data"`
}

func (h *Handler) Table(w http.ResponseWriter, r *http.Request) error {
	var season int16
	if v := r.URL.Query().Get("season"); v != "" {
		n, err := strconv.ParseInt(v, 10, 16)
		if err != nil || n < 1900 {
			return apperror.Validation("season must be a year, e.g. 2026")
		}
		season = int16(n)
	}

	season, rows, err := h.service.Table(r.Context(), season)
	if err != nil {
		return err
	}

	var resp TableResponse
	resp.Data.Season = int(season)
	resp.Data.Rows = make([]Row, 0, len(rows))
	for i, t := range rows {
		resp.Data.Rows = append(resp.Data.Rows, Row{
			Position: i + 1, TeamID: t.ID.String(), Team: t.Name, Badge: t.ShortName, Logo: t.LogoUrl.String,
			Played: t.Played, Won: t.Won, Drawn: t.Drawn, Lost: t.Lost,
			GoalsFor: t.GoalsFor, GoalsAgainst: t.GoalsAgainst, Points: t.Points, Form: t.Form,
		})
	}

	return httpx.WriteJSON(w, http.StatusOK, resp)
}
