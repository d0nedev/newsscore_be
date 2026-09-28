-- Work queue for the ingestor: one row per unit of work (kind, target), so a
-- backfill resumes where it stopped, failures back off and give up, and
-- progress is one GROUP BY away.
CREATE TABLE ingest_jobs (
    id          BIGSERIAL PRIMARY KEY,
    kind        TEXT NOT NULL,               -- match_details
    target      TEXT NOT NULL,               -- Flashscore id of what the job fetches
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts    SMALLINT NOT NULL DEFAULT 0,
    last_error  TEXT,
    not_before  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    done_at     TIMESTAMPTZ,
    UNIQUE (kind, target)
);
CREATE INDEX ingest_jobs_due_idx ON ingest_jobs (not_before, id) WHERE status IN ('pending', 'running');
