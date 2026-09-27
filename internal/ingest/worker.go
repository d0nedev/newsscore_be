package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"

	"github.com/jackc/pgx/v5/pgtype"
)

type Worker struct {
	db            *db.Queries
	logger        *slog.Logger
	detailsPerRun int
}

func NewWorker(queries *db.Queries, logger *slog.Logger, detailsPerRun int) *Worker {
	return &Worker{db: queries, logger: logger, detailsPerRun: detailsPerRun}
}

// Sync pulls the league page (fixtures and results), then fills in details for finished matches that lack them.
func (w *Worker) Sync(ctx context.Context) error {
	// Housekeeping rides along: the ingestor is the one process guaranteed to run alone.
	if n, err := w.db.DeleteExpiredSessions(ctx); err != nil {
		w.logger.Error("delete expired sessions failed", slog.Any("error", err))
	} else if n > 0 {
		w.logger.Info("expired sessions deleted", slog.Int64("count", n))
	}

	competitions, err := w.db.ListActiveCompetitions(ctx)
	if err != nil {
		return fmt.Errorf("list competitions: %w", err)
	}
	// One failing page (layout change, block) must not stop the other competitions.
	for _, c := range competitions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncCompetition(ctx, c); err != nil {
			w.logger.Error("sync competition failed", slog.String("competition", c.Slug), slog.Any("error", err))
		}
		time.Sleep(time.Second) // same politeness gap as the feed requests
	}
	return w.syncDetails(ctx)
}

func (w *Worker) syncCompetition(ctx context.Context, c db.ListActiveCompetitionsRow) error {
	matches, teams, err := flashscore.ScrapeLeague(c.FlashscorePath)
	if err != nil {
		return fmt.Errorf("scrape %s: %w", c.FlashscorePath, err)
	}

	w.logger.Info("competition scraped", slog.String("competition", c.Slug), slog.Int("teams", len(teams)), slog.Int("matches", len(matches)))

	teamIDs := make(map[string]pgtype.UUID, len(teams))
	for _, t := range teams {
		id, err := w.db.UpsertTeam(ctx, db.UpsertTeamParams{
			FlashscoreID: t.FlashscoreID,
			Name:         t.Name,
			ShortName:    t.ShortName,
			LogoUrl:      optText(t.LogoURL),
		})
		if err != nil {
			w.logger.Error("upsert team failed", slog.String("team", t.Name), slog.Any("error", err))
			continue
		}
		teamIDs[t.FlashscoreID] = id
	}

	for _, m := range matches {
		homeID, okHome := teamIDs[m.HomeTeamFlashscoreID]
		awayID, okAway := teamIDs[m.AwayTeamFlashscoreID]
		if !okHome || !okAway {
			w.logger.Warn("skip match: team not stored", slog.String("match", m.FlashscoreID))
			continue
		}

		_, err := w.db.UpsertMatch(ctx, db.UpsertMatchParams{
			FlashscoreID:   m.FlashscoreID,
			CompetitionID:  c.ID,
			Round:          optText(m.Round),
			Phase:          optText(m.Phase),
			Season:         int16(m.Season),
			HomeTeamID:     homeID,
			AwayTeamID:     awayID,
			Status:         m.Status,
			MatchTime:      pgtype.Timestamptz{Time: m.MatchTime, Valid: !m.MatchTime.IsZero()},
			HomeScore:      pgtype.Int2{Int16: int16(m.HomeScore), Valid: m.Status != "scheduled"},
			AwayScore:      pgtype.Int2{Int16: int16(m.AwayScore), Valid: m.Status != "scheduled"},
			Stage:          pgtype.Int2{Int16: int16(m.Stage), Valid: m.Stage != 0},
			StageStartedAt: pgtype.Timestamptz{Time: m.StageStartedAt, Valid: !m.StageStartedAt.IsZero()},
		})
		if err != nil {
			w.logger.Error("upsert match failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
		}
	}

	return nil
}

type statJSON struct {
	Label string `json:"label"`
	Home  string `json:"home"`
	Away  string `json:"away"`
}

func (w *Worker) syncDetails(ctx context.Context) error {
	if w.detailsPerRun == 0 {
		return nil
	}

	pending, err := w.db.ListMatchesMissingDetails(ctx, int32(w.detailsPerRun))
	if err != nil {
		return fmt.Errorf("list matches missing details: %w", err)
	}

	for _, m := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncMatchDetails(ctx, m); err != nil {
			// ponytail: a match Flashscore never serves details for is retried every run; add an attempts column if that shows up in logs.
			w.logger.Error("sync match details failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
		}
	}

	return nil
}

func (w *Worker) syncMatchDetails(ctx context.Context, m db.ListMatchesMissingDetailsRow) error {
	events, stats, players, err := flashscore.ScrapeMatchDetails(m.FlashscoreID)
	if err != nil {
		return err
	}

	if err := w.saveEvents(ctx, m.ID, m.HomeTeamID, m.AwayTeamID, events); err != nil {
		return err
	}

	for _, p := range players {
		number := pgtype.Int2{Int16: int16(p.ShirtNumber), Valid: p.ShirtNumber > 0}
		position := pgtype.Text{String: "GK", Valid: p.Goalkeeper}
		team := sideTeam(p.Team, m.HomeTeamID, m.AwayTeamID)

		playerID, err := w.db.UpsertPlayer(ctx, db.UpsertPlayerParams{
			FlashscoreID: p.FlashscoreID,
			TeamID:       team,
			Name:         p.Name,
			Nationality:  optText(p.Nationality),
			ShirtNumber:  number,
			Position:     position,
		})
		if err != nil {
			return fmt.Errorf("upsert player %s: %w", p.FlashscoreID, err)
		}
		if err := w.db.UpsertLineup(ctx, db.UpsertLineupParams{
			MatchID: m.ID, PlayerID: playerID, TeamID: team, ShirtNumber: number, Starter: p.Starter,
		}); err != nil {
			return fmt.Errorf("upsert lineup %s: %w", p.FlashscoreID, err)
		}
	}

	out := make([]statJSON, 0, len(stats))
	for _, s := range stats {
		out = append(out, statJSON{Label: s.Name, Home: s.Home, Away: s.Away})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}

	// Stats go last: their row marks the match as done, so a failure above retries it.
	return w.db.UpsertMatchStats(ctx, db.UpsertMatchStatsParams{MatchID: m.ID, Stats: raw})
}

// SyncLive refreshes score, stage, and events of matches in play. It is cheap when
// nothing is live: one indexed query and no requests to Flashscore.
func (w *Worker) SyncLive(ctx context.Context) error {
	candidates, err := w.db.ListLiveCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list live candidates: %w", err)
	}

	for _, m := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncLiveMatch(ctx, m); err != nil {
			w.logger.Error("sync live match failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
		}
	}

	return nil
}

func (w *Worker) syncLiveMatch(ctx context.Context, m db.ListLiveCandidatesRow) error {
	state, err := flashscore.ScrapeLive(m.FlashscoreID)
	if err != nil {
		return err
	}

	if state.Status != "scheduled" {
		events, err := flashscore.ScrapeEvents(m.FlashscoreID)
		if err != nil {
			return err
		}
		if err := w.saveEvents(ctx, m.ID, m.HomeTeamID, m.AwayTeamID, events); err != nil {
			return err
		}
	}

	return w.db.UpdateMatchLive(ctx, db.UpdateMatchLiveParams{
		ID:             m.ID,
		Status:         state.Status,
		Stage:          pgtype.Int2{Int16: int16(state.Stage), Valid: state.Stage != 0},
		StageStartedAt: pgtype.Timestamptz{Time: state.StageStartedAt, Valid: !state.StageStartedAt.IsZero()},
		HomeScore:      pgtype.Int2{Int16: int16(state.HomeScore), Valid: state.Status != "scheduled"},
		AwayScore:      pgtype.Int2{Int16: int16(state.AwayScore), Valid: state.Status != "scheduled"},
	})
}

func (w *Worker) saveEvents(ctx context.Context, matchID, homeID, awayID pgtype.UUID, events []flashscore.MatchEvent) error {
	for _, e := range events {
		team := sideTeam(e.Team, homeID, awayID)
		if err := w.db.UpsertMatchEvent(ctx, db.UpsertMatchEventParams{
			MatchID:             matchID,
			FlashscoreID:        e.ID,
			Type:                e.Type,
			Minute:              e.Minute,
			PlayerName:          e.PlayerName,
			TeamID:              team,
			PlayerFlashscoreID:  optText(e.PlayerID),
			RelatedPlayerName:   optText(e.RelatedName),
			RelatedFlashscoreID: optText(e.RelatedID),
		}); err != nil {
			return fmt.Errorf("upsert event %s: %w", e.ID, err)
		}
	}
	return nil
}

// sideTeam maps Flashscore's side (1 home, 2 away) to the team id.
func sideTeam(side int, homeID, awayID pgtype.UUID) pgtype.UUID {
	if side == 2 {
		return awayID
	}
	return homeID
}

func optText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
