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
	"github.com/jackc/pgx/v5/pgtype"
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

type table struct {
	Competition db.GetCompetitionRow
	Rows        []db.ListStandingsRow
	Pairings    []db.ListGroupPairingsRow // cups only
}

// Get returns a competition and its table for season; season 0 means its latest season.
func (s *Service) Get(ctx context.Context, slug string, season int16) (table, error) {
	ctx, span := s.tracer.Start(ctx, "LeagueService.Get")
	defer span.End()

	fail := func(msg string, err error) (table, error) {
		return table{}, tracing.Fail(span, apperror.Internal(CodeLeagueQueryFailed, msg, err))
	}

	c, err := s.queries.GetCompetition(ctx, slug)
	if err != nil {
		if database.IsNotFound(err) {
			return table{}, apperror.NotFound(CodeLeagueNotFound, "league not found")
		}
		return fail("failed to get league", err)
	}
	if season != 0 {
		c.Season = season
	}

	t := table{Competition: c}
	if t.Rows, err = s.queries.ListStandings(ctx, db.ListStandingsParams{CompetitionID: c.ID, Season: c.Season}); err != nil {
		return fail("failed to compute standings", err)
	}
	if c.Type == "cup" {
		if t.Pairings, err = s.queries.ListGroupPairings(ctx, db.ListGroupPairingsParams{CompetitionID: c.ID, Season: c.Season}); err != nil {
			return fail("failed to list group matches", err)
		}
	}
	return t, nil
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

// League follows the contract: id is the slug. Cups send an empty standings and one table per group.
type League struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Country   string   `json:"country"`
	CountryID string   `json:"countryId"` // country slug, e.g. "indonesia"
	Type      string   `json:"type"`
	Season    int      `json:"season"`
	Standings *[]Row   `json:"standings,omitempty"` // with tables only; a pointer so cups still send []
	Groups    *[]Group `json:"groups,omitempty"`    // cups with tables only
}

type Group struct {
	Name      string `json:"name"`
	Standings []Row  `json:"standings"`
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

// List returns every competition; ?standings=true adds each one's table, so a
// home page needs one request instead of one per league.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.service.List(r.Context())
	if err != nil {
		return err
	}
	withTables := r.URL.Query().Get("standings") == "true"

	out := make([]League, 0, len(rows))
	for _, c := range rows {
		if !withTables {
			out = append(out, League{ID: c.Slug, Name: c.Name, Country: c.Country, CountryID: c.CountrySlug, Type: c.Type, Season: int(c.Season)})
			continue
		}
		// ponytail: one table query per competition; fine for a handful, batch them if the list grows past ~20.
		t, err := h.service.Get(r.Context(), c.Slug, 0)
		if err != nil {
			return err
		}
		out = append(out, withTable(t))
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

	t, err := h.service.Get(r.Context(), chi.URLParam(r, "slug"), season)
	if err != nil {
		return err
	}

	return httpx.WriteJSON(w, http.StatusOK, struct {
		Data League `json:"data"`
	}{withTable(t)})
}

// withTable renders a competition with its standings, or its groups for a cup.
func withTable(t table) League {
	c := t.Competition
	standings := []Row{}
	out := League{ID: c.Slug, Name: c.Name, Country: c.Country, CountryID: c.CountrySlug, Type: c.Type, Season: int(c.Season), Standings: &standings}
	if c.Type == "league" {
		standings = toRows(t.Rows)
	} else {
		groups := splitGroups(t.Rows, t.Pairings)
		out.Groups = &groups
	}
	return out
}

func toRows(rows []db.ListStandingsRow) []Row {
	out := make([]Row, 0, len(rows))
	for i, t := range rows {
		out = append(out, Row{
			Position: i + 1, TeamID: t.ID.String(), Team: t.Name, Badge: t.ShortName, Logo: t.LogoUrl.String,
			Played: t.Played, Won: t.Won, Drawn: t.Drawn, Lost: t.Lost,
			GoalsFor: t.GoalsFor, GoalsAgainst: t.GoalsAgainst, Points: t.Points, Form: t.Form,
		})
	}
	return out
}

// splitGroups derives a cup's groups from who played whom: Flashscore does not label
// group matches, but teams in one group only meet each other, so each connected
// component of the group-stage fixtures is a group. rows arrive ranked, so each
// group keeps the ranking. Groups are named A, B, … by their best-ranked team.
func splitGroups(rows []db.ListStandingsRow, pairings []db.ListGroupPairingsRow) []Group {
	parent := map[pgtype.UUID]pgtype.UUID{}
	var find func(pgtype.UUID) pgtype.UUID
	find = func(x pgtype.UUID) pgtype.UUID {
		p, ok := parent[x]
		if !ok || p == x {
			return x
		}
		root := find(p)
		parent[x] = root
		return root
	}
	for _, p := range pairings {
		parent[find(p.HomeTeamID)] = find(p.AwayTeamID)
	}

	index := map[pgtype.UUID]int{}
	var groups [][]db.ListStandingsRow
	for _, row := range rows {
		root := find(row.ID)
		i, ok := index[root]
		if !ok {
			i = len(groups)
			index[root] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], row)
	}

	out := make([]Group, 0, len(groups))
	for i, g := range groups {
		out = append(out, Group{Name: "Grup " + string(rune('A'+i)), Standings: toRows(g)})
	}
	return out
}

// Scope resolves an optional ?leagueId= for pages that span competitions (team,
// player): with a slug it is that competition and its latest season; without,
// every competition in the latest season overall (competition id NULL).
func Scope(ctx context.Context, queries *db.Queries, slug string) (pgtype.UUID, int16, error) {
	if slug == "" {
		season, err := queries.LatestSeason(ctx)
		return pgtype.UUID{}, season, err
	}
	c, err := queries.GetCompetition(ctx, slug)
	if database.IsNotFound(err) {
		return pgtype.UUID{}, 0, apperror.NotFound(CodeLeagueNotFound, "league not found")
	}
	return c.ID, c.Season, err
}
