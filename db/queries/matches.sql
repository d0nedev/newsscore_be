-- name: ListMatches :many
SELECT * FROM match_rows
WHERE match_time >= sqlc.arg(from_time) AND match_time < sqlc.arg(to_time)
  AND (sqlc.narg(team_id)::uuid IS NULL OR sqlc.narg(team_id)::uuid IN (home_team_id, away_team_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(competition)::text IS NULL OR competition_slug = sqlc.narg(competition)::text)
ORDER BY competition_sort, match_time, id;

-- name: GetMatch :one
SELECT * FROM match_rows WHERE id = $1;

-- name: ListMatchEvents :many
SELECT type, minute, player_name, team_id, related_player_name
FROM match_events
WHERE match_id = $1
ORDER BY NULLIF(substring(minute FROM '^\d+'), '')::int NULLS LAST, id;

-- name: GetMatchStats :one
SELECT stats FROM match_statistics WHERE match_id = $1;

-- name: ListMatchLineups :many
SELECT p.id, p.name, l.shirt_number, l.starter, l.team_id
FROM match_lineups l
JOIN players p ON p.id = l.player_id
WHERE l.match_id = $1
ORDER BY l.starter DESC, l.shirt_number NULLS LAST, p.name;
