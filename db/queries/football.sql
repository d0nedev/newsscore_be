-- name: UpsertTeam :one
INSERT INTO teams (flashscore_id, name, short_name, logo_url, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (flashscore_id) DO UPDATE
SET name = EXCLUDED.name,
    short_name = EXCLUDED.short_name,
    logo_url = EXCLUDED.logo_url,
    updated_at = now()
RETURNING id;

-- name: UpsertMatch :one
INSERT INTO matches (flashscore_id, season, home_team_id, away_team_id, status, match_time, home_score, away_score, data_as_of, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
ON CONFLICT (flashscore_id) DO UPDATE
SET status = EXCLUDED.status,
    match_time = EXCLUDED.match_time,
    home_score = EXCLUDED.home_score,
    away_score = EXCLUDED.away_score,
    data_as_of = now(),
    updated_at = now()
RETURNING id;

-- name: UpsertStanding :exec
INSERT INTO standings (season, team_id, rank, points, form, zone, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (season, team_id) DO UPDATE
SET rank = EXCLUDED.rank,
    points = EXCLUDED.points,
    form = EXCLUDED.form,
    zone = EXCLUDED.zone,
    updated_at = now();
