// Package player serves player pages: profile, season totals, and match log.
package player

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"uuid"

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
	CodePlayerNotFound    = "PLAYER_NOT_FOUND"
	CodePlayerQueryFailed = "PLAYER_QUERY_FAILED"
)

var wib = time.FixedZone("WIB", 7*60*60)

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

func (s *Service) Profile(ctx context.Context, id uuid.UUID) (db.GetPlayerRow, int16, []db.ListPlayerMatchesRow, error) {
	ctx, span := s.tracer.Start(ctx, "PlayerService.Profile")
	defer span.End()

	fail := func(msg string, err error) error {
		return tracing.Fail(span, apperror.Internal(CodePlayerQueryFailed, msg, err))
	}

	p, err := s.queries.GetPlayer(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		if database.IsNotFound(err) {
			return p, 0, nil, apperror.NotFound(CodePlayerNotFound, "player not found")
		}
		return p, 0, nil, fail("failed to get player", err)
	}
	season, err := s.queries.LatestSeason(ctx)
	if err != nil {
		return p, 0, nil, fail("failed to find season", err)
	}
	log, err := s.queries.ListPlayerMatches(ctx, db.ListPlayerMatchesParams{
		FlashscoreID: p.FlashscoreID, PlayerID: p.ID, Season: season,
	})
	if err != nil {
		return p, 0, nil, fail("failed to list matches", err)
	}
	return p, season, log, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r chi.Router, h *Handler, logger *slog.Logger) {
	r.Get("/players/{id}", httpx.Handle(logger, h.Get))
}

type Team struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Badge string `json:"badge"`
	Logo  string `json:"logo,omitempty"`
}

type SeasonTotals struct {
	Season  int   `json:"season"`
	Matches int32 `json:"matches"`
	Starts  int32 `json:"starts"`
	Goals   int32 `json:"goals"`
	Assists int32 `json:"assists"`
	Yellow  int32 `json:"yellow"`
	Red     int32 `json:"red"`
}

type MatchLogEntry struct {
	Date    string  `json:"date"`
	MatchID string  `json:"matchId"`
	Home    string  `json:"home"`
	Away    string  `json:"away"`
	Score   *[2]int `json:"score"`
	Outcome string  `json:"outcome,omitempty"` // W/D/L from the player's side; empty until finished
	Started bool    `json:"started"`
	Played  bool    `json:"played"` // false: on the bench and never came on
	Goals   int32   `json:"goals"`
	Assists int32   `json:"assists"`
	Yellow  int32   `json:"yellow"`
	Red     int32   `json:"red"`
}

type Profile struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Number   int             `json:"number,omitempty"`
	Position string          `json:"position,omitempty"`
	Country  string          `json:"country,omitempty"`
	Team     *Team           `json:"team"`
	Season   SeasonTotals    `json:"season"`
	MatchLog []MatchLogEntry `json:"matchLog"`
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return apperror.Validation("invalid player id")
	}

	p, season, log, err := h.service.Profile(r.Context(), id)
	if err != nil {
		return err
	}

	out := Profile{
		ID: p.ID.String(), Name: p.Name, Number: int(p.ShirtNumber.Int16),
		Position: p.Position.String, Country: p.Nationality.String,
		Season:   SeasonTotals{Season: int(season)},
		MatchLog: make([]MatchLogEntry, 0, len(log)),
	}
	if p.TeamID.Valid {
		out.Team = &Team{ID: p.TeamID.String(), Name: p.TeamName.String, Badge: p.TeamShortName.String, Logo: p.TeamLogoUrl.String}
	}

	for _, m := range log {
		e := MatchLogEntry{
			Date: m.MatchTime.Time.In(wib).Format("02.01.2006"), MatchID: m.ID.String(),
			Home: m.HomeName, Away: m.AwayName,
			Started: m.Starter, Played: m.Starter || m.SubbedOn,
			Goals: m.Goals, Assists: m.Assists, Yellow: m.Yellow, Red: m.Red,
		}
		if m.Status == "finished" && m.HomeScore.Valid && m.AwayScore.Valid {
			home, away := int(m.HomeScore.Int16), int(m.AwayScore.Int16)
			e.Score = &[2]int{home, away}
			e.Outcome = outcome(home, away, m.TeamID == m.HomeTeamID)
		}
		out.MatchLog = append(out.MatchLog, e)

		if e.Played {
			out.Season.Matches++
		}
		if e.Started {
			out.Season.Starts++
		}
		out.Season.Goals += m.Goals
		out.Season.Assists += m.Assists
		out.Season.Yellow += m.Yellow
		out.Season.Red += m.Red
	}

	return httpx.WriteJSON(w, http.StatusOK, struct {
		Data Profile `json:"data"`
	}{out})
}

func outcome(home, away int, isHome bool) string {
	own, other := home, away
	if !isHome {
		own, other = away, home
	}
	switch {
	case own > other:
		return "W"
	case own < other:
		return "L"
	}
	return "D"
}
