-- name: UpsertTeam :one
INSERT INTO teams (flashscore_id, name, short_name, logo_source_url, country_id, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (flashscore_id) DO UPDATE
SET name = EXCLUDED.name,
    -- Only national team pages know a team's country; a league page never clears it.
    country_id = coalesce(EXCLUDED.country_id, teams.country_id),
    short_name = EXCLUDED.short_name,
    -- A new source clears the stored copy so the ingestor downloads it again.
    logo_url = CASE WHEN teams.logo_source_url IS DISTINCT FROM EXCLUDED.logo_source_url THEN NULL ELSE teams.logo_url END,
    logo_source_url = EXCLUDED.logo_source_url,
    updated_at = now()
RETURNING id;

-- name: UpsertMatch :one
INSERT INTO matches (flashscore_id, competition_id, season, home_team_id, away_team_id, status, match_time, home_score, away_score, stage, stage_started_at, round, phase, data_as_of, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now())
ON CONFLICT (flashscore_id) DO UPDATE
SET status = EXCLUDED.status,
    round = EXCLUDED.round,
    phase = EXCLUDED.phase,
    season = EXCLUDED.season,
    stage = EXCLUDED.stage,
    stage_started_at = EXCLUDED.stage_started_at,
    match_time = EXCLUDED.match_time,
    home_score = EXCLUDED.home_score,
    away_score = EXCLUDED.away_score,
    data_as_of = now(),
    updated_at = now()
RETURNING id;


-- name: ListLiveCandidates :many
-- Live matches, plus scheduled ones whose kick-off has passed but the league page has not caught up.
SELECT id, flashscore_id, home_team_id, away_team_id
FROM matches
WHERE status = 'live'
   OR (status = 'scheduled' AND match_time <= now() AND match_time > now() - interval '3 hours');

-- name: UpdateMatchLive :exec
UPDATE matches
SET status = $2, stage = $3, stage_started_at = $4, home_score = $5, away_score = $6,
    data_as_of = now(), updated_at = now()
WHERE id = $1;

-- name: ListActiveCompetitions :many
SELECT id, slug, flashscore_path FROM competitions WHERE active AND scraped ORDER BY sort_order;

-- name: ListNationalTeamsToScrape :many
-- National teams of countries with a scraped league, e.g. Indonesia.
SELECT t.flashscore_id, n.slug AS country_slug
FROM teams t
JOIN countries n ON n.id = t.country_id
WHERE EXISTS (SELECT 1 FROM competitions c WHERE c.country_id = n.id AND c.active AND c.scraped);

-- name: UpsertCountry :one
INSERT INTO countries (slug, name) VALUES ($1, $2)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id;

-- name: UpsertTeamPageCompetition :one
-- A competition seen on a national team's page; see migration 0020.
INSERT INTO competitions (slug, name, country_id, type, flashscore_path, scraped, sort_order)
VALUES ($1, $2, $3, 'cup', $4, false, 100)
ON CONFLICT (flashscore_path) DO UPDATE SET name = EXCLUDED.name
RETURNING id;

-- name: ListTeamsMissingLogo :many
SELECT id, flashscore_id, logo_source_url::text AS logo_source_url
FROM teams
WHERE logo_url IS NULL AND logo_source_url IS NOT NULL;

-- name: SetTeamLogo :exec
UPDATE teams SET logo_url = $2 WHERE id = $1;
