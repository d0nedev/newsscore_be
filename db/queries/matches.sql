-- name: ListMatches :many
SELECT m.id, m.status, m.match_time, m.home_score, m.away_score,
       h.id AS home_id, h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.id AS away_id, a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
WHERE m.match_time >= sqlc.arg(from_time) AND m.match_time < sqlc.arg(to_time)
  AND (sqlc.narg(team_id)::uuid IS NULL OR sqlc.narg(team_id)::uuid IN (m.home_team_id, m.away_team_id))
  AND (sqlc.narg(status)::text IS NULL OR m.status = sqlc.narg(status)::text)
ORDER BY m.match_time, m.id;

-- name: GetMatch :one
SELECT m.id, m.status, m.match_time, m.home_score, m.away_score,
       h.id AS home_id, h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.id AS away_id, a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
WHERE m.id = $1;

-- name: ListMatchEvents :many
SELECT type, minute, player_name, team_id
FROM match_events
WHERE match_id = $1
ORDER BY NULLIF(substring(minute FROM '^\d+'), '')::int NULLS LAST, id;

-- name: GetMatchStats :one
SELECT stats FROM match_statistics WHERE match_id = $1;
