-- name: UpsertPlayer :one
-- Position is only known for goalkeepers (lineup role); never downgrade a known one to NULL.
INSERT INTO players (flashscore_id, team_id, name, nationality, shirt_number, position, photo_source_url, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (flashscore_id) DO UPDATE
SET name = EXCLUDED.name,
    nationality = coalesce(EXCLUDED.nationality, players.nationality),
    -- team_id is the club: a national team match keeps the club the player already has.
    team_id = CASE WHEN (SELECT country_id FROM teams WHERE id = EXCLUDED.team_id) IS NULL
                   THEN EXCLUDED.team_id ELSE players.team_id END,
    shirt_number = coalesce(EXCLUDED.shirt_number, players.shirt_number),
    position = coalesce(EXCLUDED.position, players.position),
    -- A new photo source clears the stored copy so the ingestor downloads it again.
    photo_url = CASE WHEN EXCLUDED.photo_source_url IS NOT NULL AND EXCLUDED.photo_source_url IS DISTINCT FROM players.photo_source_url
                     THEN NULL ELSE players.photo_url END,
    photo_source_url = coalesce(EXCLUDED.photo_source_url, players.photo_source_url),
    updated_at = now()
RETURNING id;

-- name: UpsertLineup :exec
INSERT INTO match_lineups (match_id, player_id, team_id, shirt_number, starter)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (match_id, player_id) DO UPDATE
SET team_id = EXCLUDED.team_id, shirt_number = EXCLUDED.shirt_number, starter = EXCLUDED.starter;

-- name: UpsertMatchEvent :exec
INSERT INTO match_events (match_id, flashscore_id, type, minute, player_name, team_id,
                          player_flashscore_id, related_player_name, related_flashscore_id, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (match_id, flashscore_id) DO UPDATE
SET type = EXCLUDED.type,
    minute = EXCLUDED.minute,
    player_name = EXCLUDED.player_name,
    team_id = EXCLUDED.team_id,
    player_flashscore_id = EXCLUDED.player_flashscore_id,
    related_player_name = EXCLUDED.related_player_name,
    related_flashscore_id = EXCLUDED.related_flashscore_id,
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

-- name: ListPlayersMissingPhoto :many
SELECT id, flashscore_id, photo_source_url::text AS photo_source_url
FROM players
WHERE photo_url IS NULL AND photo_source_url IS NOT NULL
LIMIT $1;

-- name: SetPlayerPhoto :exec
UPDATE players SET photo_url = $2 WHERE id = $1;
