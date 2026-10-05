-- name: EnqueueMatchDetails :execrows
-- Finished matches without details in their competition's current season,
-- newest first: club pages also bring last season's results, kept for scores
-- only. A done job whose details are gone again (stats cleared by a migration)
-- is queued anew; a failed one stays failed.
INSERT INTO ingest_jobs (kind, target)
SELECT 'match_details', m.flashscore_id
FROM matches m
WHERE m.status = 'finished'
  AND m.flashscore_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM match_statistics s WHERE s.match_id = m.id)
  AND m.season >= (SELECT max(season) FROM matches cur
                   WHERE cur.competition_id = m.competition_id AND cur.status = 'finished')
ORDER BY m.match_time DESC
ON CONFLICT (kind, target) DO UPDATE
SET status = 'pending', attempts = 0, not_before = now(), updated_at = now()
WHERE ingest_jobs.status = 'done';

-- name: ClaimJobs :many
-- Due jobs, oldest first, plus running ones a crashed ingestor left behind.
-- SKIP LOCKED keeps two ingestors from taking the same job.
UPDATE ingest_jobs
SET status = 'running', attempts = attempts + 1, updated_at = now()
WHERE id IN (
    SELECT id FROM ingest_jobs
    WHERE (status = 'pending' AND not_before <= now())
       OR (status = 'running' AND updated_at < now() - interval '15 minutes')
    ORDER BY not_before, id
    LIMIT $1
    FOR UPDATE SKIP LOCKED)
RETURNING id, kind, target, attempts;

-- name: FinishJob :exec
UPDATE ingest_jobs SET status = 'done', last_error = NULL, done_at = now(), updated_at = now() WHERE id = $1;

-- name: RetryJob :exec
-- Back to the queue after delay_seconds; give_up marks it failed for good.
UPDATE ingest_jobs
SET status = CASE WHEN sqlc.arg(give_up)::bool THEN 'failed' ELSE 'pending' END,
    last_error = sqlc.arg(last_error),
    not_before = now() + make_interval(secs => sqlc.arg(delay_seconds)::float8),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ReleaseJob :exec
-- Back to the queue without spending an attempt (the source blocked us, not the job's fault).
UPDATE ingest_jobs
SET status = 'pending', attempts = attempts - 1,
    not_before = now() + make_interval(secs => sqlc.arg(delay_seconds)::float8), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: JobProgress :many
SELECT kind, status, count(*)::int AS jobs FROM ingest_jobs GROUP BY kind, status ORDER BY kind, status;

-- name: EnqueueTeamPages :execrows
-- Every club in a scraped competition, once a week.
INSERT INTO ingest_jobs (kind, target)
SELECT 'team_page', t.flashscore_id
FROM teams t
WHERE t.country_id IS NULL
  AND t.flashscore_id IS NOT NULL
  AND t.flashscore_slug IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM matches m JOIN competitions c ON c.id = m.competition_id
      WHERE c.active AND c.scraped AND t.id IN (m.home_team_id, m.away_team_id))
ON CONFLICT (kind, target) DO UPDATE
SET status = 'pending', attempts = 0, not_before = now(), updated_at = now()
WHERE ingest_jobs.status = 'done' AND ingest_jobs.done_at < now() - interval '7 days';
