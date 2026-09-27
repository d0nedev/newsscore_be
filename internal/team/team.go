// Package team serves team pages: profile, squad with season numbers, recent and upcoming matches.
package team

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/d0nedev/newsscore/internal/league"
	"github.com/d0nedev/newsscore/internal/match"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

const (
	CodeTeamNotFound    = "TEAM_NOT_FOUND"
	CodeTeamQueryFailed = "TEAM_QUERY_FAILED"
)

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

type profile struct {
	Team     db.GetTeamRow
	Season   int16
	Squad    []db.ListTeamSquadRow
	Recent   []db.MatchRow
	Upcoming []db.MatchRow
}

// Profile covers every competition, or only leagueSlug when set.
func (s *Service) Profile(ctx context.Context, id uuid.UUID, leagueSlug string) (profile, error) {
	ctx, span := s.tracer.Start(ctx, "TeamService.Profile")
	defer span.End()

	fail := func(msg string, err error) (profile, error) {
		return profile{}, tracing.Fail(span, apperror.Internal(CodeTeamQueryFailed, msg, err))
	}
	teamID := database.UUID(id)

	t, err := s.queries.GetTeam(ctx, teamID)
	if err != nil {
		if database.IsNotFound(err) {
			return profile{}, apperror.NotFound(CodeTeamNotFound, "team not found")
		}
		return fail("failed to get team", err)
	}
	competition, season, err := league.Scope(ctx, s.queries, leagueSlug)
	if err != nil {
		if _, ok := errors.AsType[*apperror.Error](err); ok {
			return profile{}, err
		}
		return fail("failed to find season", err)
	}
	squad, err := s.queries.ListTeamSquad(ctx, db.ListTeamSquadParams{Season: season, TeamID: teamID, CompetitionID: competition})
	if err != nil {
		return fail("failed to list squad", err)
	}
	recent, err := s.queries.ListTeamRecentMatches(ctx, db.ListTeamRecentMatchesParams{TeamID: teamID, CompetitionID: competition})
	if err != nil {
		return fail("failed to list matches", err)
	}
	upcoming, err := s.queries.ListTeamUpcomingMatches(ctx, db.ListTeamUpcomingMatchesParams{TeamID: teamID, CompetitionID: competition})
	if err != nil {
		return fail("failed to list matches", err)
	}

	return profile{Team: t, Season: season, Squad: squad, Recent: recent, Upcoming: upcoming}, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger) {
	r.Get("/teams/{id}", httpx.Handle(logger, h.Get))
}

type SquadPlayer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Number      int    `json:"number,omitempty"`
	Position    string `json:"position,omitempty"` // only "GK" is known from the source
	Nationality string `json:"nationality,omitempty"`
	Matches     int32  `json:"matches"`
	Goals       int32  `json:"goals"`
	Assists     int32  `json:"assists"`
}

type Profile struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Badge    string                `json:"badge"`
	Logo     string                `json:"logo,omitempty"`
	Season   int                   `json:"season"`
	Squad    []SquadPlayer         `json:"squad"`
	Recent   []match.MatchResponse `json:"recentMatches"`
	Upcoming []match.MatchResponse `json:"upcomingMatches"`
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return apperror.Validation("invalid team id")
	}

	p, err := h.service.Profile(r.Context(), id, r.URL.Query().Get("leagueId"))
	if err != nil {
		return err
	}

	out := Profile{
		ID: p.Team.ID.String(), Name: p.Team.Name, Badge: p.Team.ShortName, Logo: p.Team.LogoUrl.String,
		Season:   int(p.Season),
		Squad:    make([]SquadPlayer, 0, len(p.Squad)),
		Recent:   make([]match.MatchResponse, 0, len(p.Recent)),
		Upcoming: make([]match.MatchResponse, 0, len(p.Upcoming)),
	}
	for _, s := range p.Squad {
		out.Squad = append(out.Squad, SquadPlayer{
			ID: s.ID.String(), Name: s.Name, Number: int(s.ShirtNumber.Int16),
			Position: s.Position.String, Nationality: s.Nationality.String,
			Matches: s.Apps, Goals: s.Goals, Assists: s.Assists,
		})
	}
	for _, m := range p.Recent {
		out.Recent = append(out.Recent, match.ToMatchResponse(m))
	}
	for _, m := range p.Upcoming {
		out.Upcoming = append(out.Upcoming, match.ToMatchResponse(m))
	}

	return httpx.WriteJSON(w, http.StatusOK, struct {
		Data Profile `json:"data"`
	}{out})
}
