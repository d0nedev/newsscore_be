package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

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

	if err := w.syncLeague(ctx); err != nil {
		return err
	}
	return w.syncDetails(ctx)
}

func (w *Worker) syncLeague(ctx context.Context) error {
	matches, teams, err := flashscore.ScrapeLeague()
	if err != nil {
		return fmt.Errorf("scrape league: %w", err)
	}

	w.logger.Info("league scraped", slog.Int("teams", len(teams)), slog.Int("matches", len(matches)))

	teamIDs := make(map[string]pgtype.UUID, len(teams))
	for _, t := range teams {
		id, err := w.db.UpsertTeam(ctx, db.UpsertTeamParams{
			FlashscoreID: t.FlashscoreID,
			Name:         t.Name,
			ShortName:    t.ShortName,
			LogoUrl:      pgtype.Text{String: t.LogoURL, Valid: t.LogoURL != ""},
		})
		if err != nil {
			w.logger.Error("upsert team failed", slog.String("team", t.Name), slog.Any("error", err))
			continue
		}
		teamIDs[t.FlashscoreID] = id
	}

	stats := make(map[pgtype.UUID]*db.UpsertStandingParams)

	for _, m := range matches {
		homeID, okHome := teamIDs[m.HomeTeamFlashscoreID]
		awayID, okAway := teamIDs[m.AwayTeamFlashscoreID]
		if !okHome || !okAway {
			w.logger.Warn("skip match: team not stored", slog.String("match", m.FlashscoreID))
			continue
		}
		season := int16(m.Season)

		_, err := w.db.UpsertMatch(ctx, db.UpsertMatchParams{
			FlashscoreID:   m.FlashscoreID,
			Season:         season,
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
			continue
		}

		if m.Status == "finished" {
			if stats[homeID] == nil {
				stats[homeID] = &db.UpsertStandingParams{TeamID: homeID, Season: season}
			}
			if stats[awayID] == nil {
				stats[awayID] = &db.UpsertStandingParams{TeamID: awayID, Season: season}
			}

			sh := stats[homeID]
			sa := stats[awayID]

			if m.HomeScore > m.AwayScore {
				sh.Points += 3
			} else if m.HomeScore < m.AwayScore {
				sa.Points += 3
			} else {
				sh.Points += 1
				sa.Points += 1
			}
		}
	}

	var standings []*db.UpsertStandingParams
	for _, v := range stats {
		standings = append(standings, v)
	}

	sort.Slice(standings, func(i, j int) bool {
		return standings[i].Points > standings[j].Points
	})

	for i, st := range standings {
		st.Rank = int16(i + 1)
		if err := w.db.UpsertStanding(ctx, *st); err != nil {
			w.logger.Error("upsert standing failed", slog.String("team", st.TeamID.String()), slog.Any("error", err))
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

	teamBySide := func(side int) pgtype.UUID {
		if side == 2 {
			return m.AwayTeamID
		}
		return m.HomeTeamID
	}

	if err := w.saveEvents(ctx, m.ID, m.HomeTeamID, m.AwayTeamID, events); err != nil {
		return err
	}

	for _, p := range players {
		if _, err := w.db.UpsertPlayer(ctx, db.UpsertPlayerParams{
			FlashscoreID: p.FlashscoreID,
			TeamID:       teamBySide(p.Team),
			Name:         p.Name,
			Nationality:  pgtype.Text{String: p.Nationality, Valid: p.Nationality != ""},
		}); err != nil {
			return fmt.Errorf("upsert player %s: %w", p.FlashscoreID, err)
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
		team := homeID
		if e.Team == 2 {
			team = awayID
		}
		if err := w.db.UpsertMatchEvent(ctx, db.UpsertMatchEventParams{
			MatchID:      matchID,
			FlashscoreID: e.ID,
			Type:         e.Type,
			Minute:       e.Minute,
			PlayerName:   e.PlayerName,
			TeamID:       team,
		}); err != nil {
			return fmt.Errorf("upsert event %s: %w", e.ID, err)
		}
	}
	return nil
}
