package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/d0nedev/newsscore/internal/domain"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxJobAttempts = 5
	firstRetry     = 5 * time.Minute  // doubles per attempt: 5, 10, 20, 40 minutes
	blockedPause   = 15 * time.Minute // after a 403/429, every claimed job waits this long
)

// runJobs queues finished matches without details and each club's page once a
// week, then works through up to detailsPerRun due jobs, oldest first.
// Progress: SELECT kind, status, count(*) FROM ingest_jobs GROUP BY 1, 2.
func (w *Worker) runJobs(ctx context.Context) error {
	if w.detailsPerRun == 0 {
		return nil
	}
	if n, err := w.db.EnqueueMatchDetails(ctx); err != nil {
		return fmt.Errorf("enqueue match details: %w", err)
	} else if n > 0 {
		w.logger.Info("jobs queued", slog.String("kind", "match_details"), slog.Int64("count", n))
	}
	if n, err := w.db.EnqueueTeamPages(ctx); err != nil {
		return fmt.Errorf("enqueue team pages: %w", err)
	} else if n > 0 {
		w.logger.Info("jobs queued", slog.String("kind", "team_page"), slog.Int64("count", n))
	}

	jobs, err := w.db.ClaimJobs(ctx, int32(w.detailsPerRun))
	if err != nil {
		return fmt.Errorf("claim jobs: %w", err)
	}
	for i, j := range jobs {
		if ctx.Err() != nil {
			// Shutting down: leave the rest running; ClaimJobs picks them up again after 15 minutes.
			return ctx.Err()
		}
		err := w.runJob(ctx, j)
		switch {
		case err == nil:
			err = w.db.FinishJob(ctx, j.ID)
		case errors.Is(err, flashscore.ErrBlocked):
			w.logger.Error("flashscore blocked us, pausing jobs", slog.Duration("pause", blockedPause), slog.Any("error", err))
			for _, rest := range jobs[i:] {
				if err := w.db.ReleaseJob(ctx, db.ReleaseJobParams{ID: rest.ID, DelaySeconds: blockedPause.Seconds()}); err != nil {
					return err
				}
			}
			return w.logProgress(ctx)
		default:
			giveUp := j.Attempts >= maxJobAttempts
			w.logger.Warn("job failed", slog.String("kind", j.Kind), slog.String("target", j.Target),
				slog.Int("attempt", int(j.Attempts)), slog.Bool("gave_up", giveUp), slog.Any("error", err))
			err = w.db.RetryJob(ctx, db.RetryJobParams{
				ID:           j.ID,
				GiveUp:       giveUp,
				LastError:    optText(err.Error()),
				DelaySeconds: (firstRetry << (j.Attempts - 1)).Seconds(),
			})
		}
		if err != nil {
			return err
		}
	}
	return w.logProgress(ctx)
}

func (w *Worker) runJob(ctx context.Context, j db.ClaimJobsRow) error {
	switch j.Kind {
	case "match_details":
		m, err := w.db.GetMatchByFlashscoreID(ctx, j.Target)
		if err != nil {
			return fmt.Errorf("get match: %w", err)
		}
		return w.syncMatchDetails(ctx, m)
	case "team_page":
		return w.syncTeamPage(ctx, j.Target)
	}
	return fmt.Errorf("unknown job kind %q", j.Kind)
}

// syncTeamPage stores a club's results and fixtures in the competitions we
// scrape; a league page shows only 100 of each. Friendlies and other
// competitions on the page are skipped.
func (w *Worker) syncTeamPage(ctx context.Context, flashscoreID string) error {
	t, err := w.db.GetTeamPage(ctx, optText(flashscoreID))
	if err != nil {
		return fmt.Errorf("get team: %w", err)
	}
	competitions, err := w.db.ListScrapedCompetitionPaths(ctx)
	if err != nil {
		return err
	}
	byPath := make(map[string]db.ListScrapedCompetitionPathsRow, len(competitions))
	for _, c := range competitions {
		byPath[c.FlashscorePath] = c
	}

	time.Sleep(time.Second)
	matches, teams, err := flashscore.ScrapeTeam("/team/" + t.FlashscoreSlug + "/" + flashscoreID + "/results/")
	if err != nil {
		return err
	}

	var kept []domain.Match
	needed := map[string]bool{}
	for _, m := range matches {
		if _, ok := byPath[m.Competition.Path]; ok {
			kept = append(kept, m)
			needed[m.HomeTeamFlashscoreID], needed[m.AwayTeamFlashscoreID] = true, true
		}
	}
	var opponents []domain.Team
	for _, o := range teams {
		if needed[o.FlashscoreID] {
			opponents = append(opponents, o)
		}
	}
	teamIDs := w.upsertTeams(ctx, opponents, false)

	for _, m := range kept {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c := byPath[m.Competition.Path]
		home, okHome := teamIDs[m.HomeTeamFlashscoreID]
		away, okAway := teamIDs[m.AwayTeamFlashscoreID]
		if !okHome || !okAway {
			continue // upsertTeams logged why
		}
		if err := w.db.UpsertTeamPageMatch(ctx, db.UpsertTeamPageMatchParams{
			FlashscoreID:   m.FlashscoreID,
			CompetitionID:  c.ID,
			Season:         int16(flashscore.SeasonOf(m.MatchTime, c.SplitSeason)),
			HomeTeamID:     home,
			AwayTeamID:     away,
			Status:         m.Status,
			MatchTime:      pgtype.Timestamptz{Time: m.MatchTime, Valid: !m.MatchTime.IsZero()},
			HomeScore:      pgtype.Int2{Int16: int16(m.HomeScore), Valid: m.Status != "scheduled"},
			AwayScore:      pgtype.Int2{Int16: int16(m.AwayScore), Valid: m.Status != "scheduled"},
			Stage:          pgtype.Int2{Int16: int16(m.Stage), Valid: m.Stage != 0},
			StageStartedAt: pgtype.Timestamptz{Time: m.StageStartedAt, Valid: !m.StageStartedAt.IsZero()},
			Round:          optText(m.Round),
			Phase:          optText(m.Phase),
		}); err != nil {
			return fmt.Errorf("upsert match %s: %w", m.FlashscoreID, err)
		}
	}
	w.logger.Info("team page synced", slog.String("team", t.FlashscoreSlug), slog.Int("matches", len(kept)))
	return nil
}

func (w *Worker) logProgress(ctx context.Context) error {
	rows, err := w.db.JobProgress(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		w.logger.Info("job progress", slog.String("kind", r.Kind), slog.String("status", r.Status), slog.Int("jobs", int(r.Jobs)))
	}
	return nil
}
