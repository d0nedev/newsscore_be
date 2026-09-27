-- name: UpsertPlayer :one
INSERT INTO players (flashscore_id, team_id, name, nationality, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (flashscore_id) DO UPDATE
SET name = EXCLUDED.name,
    nationality = EXCLUDED.nationality,
    team_id = EXCLUDED.team_id,
    updated_at = now()
RETURNING id;

-- name: UpsertMatchEvent :exec
INSERT INTO match_events (match_id, flashscore_id, type, minute, player_name, team_id, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (match_id, flashscore_id) DO UPDATE
SET type = EXCLUDED.type,
    minute = EXCLUDED.minute,
    player_name = EXCLUDED.player_name,
    team_id = EXCLUDED.team_id,
    updated_at = now();

-- name: UpsertMatchStats :exec
INSERT INTO match_statistics (match_id, stats, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (match_id) DO UPDATE
SET stats = EXCLUDED.stats,
    updated_at = now();

-- name: ListMatchesMissingDetails :many
-- Newest finished matches first, so a stubborn old match cannot block fresh ones.
SELECT m.id, m.flashscore_id, m.home_team_id, m.away_team_id
FROM matches m
WHERE m.status = 'finished'
  AND NOT EXISTS (SELECT 1 FROM match_statistics s WHERE s.match_id = m.id)
ORDER BY m.match_time DESC
LIMIT $1;
