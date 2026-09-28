package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"
)

const (
	maxJobAttempts = 5
	firstRetry     = 5 * time.Minute  // doubles per attempt: 5, 10, 20, 40 minutes
	blockedPause   = 15 * time.Minute // after a 403/429, every claimed job waits this long
)

// runJobs queues finished matches without details, then works through up to
// detailsPerRun due jobs. Progress: SELECT kind, status, count(*) FROM ingest_jobs GROUP BY 1, 2.
func (w *Worker) runJobs(ctx context.Context) error {
	if w.detailsPerRun == 0 {
		return nil
	}
	if n, err := w.db.EnqueueMatchDetails(ctx); err != nil {
		return fmt.Errorf("enqueue match details: %w", err)
	} else if n > 0 {
		w.logger.Info("jobs queued", slog.String("kind", "match_details"), slog.Int64("count", n))
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
	}
	return fmt.Errorf("unknown job kind %q", j.Kind)
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
