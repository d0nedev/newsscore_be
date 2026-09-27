-- Live minute is derived at read time from the current stage and when it started,
-- so it stays accurate between scrapes.
ALTER TABLE matches
    ADD COLUMN stage            SMALLINT,
    ADD COLUMN stage_started_at TIMESTAMPTZ;

CREATE INDEX matches_live_candidates ON matches (match_time) WHERE status <> 'finished';
