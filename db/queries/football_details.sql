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
-- stats holds Flashscore's player rating: {"rating": 7.1}.
INSERT INTO match_lineups (match_id, player_id, team_id, shirt_number, starter, formation_place, stats)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (match_id, player_id) DO UPDATE
SET team_id = EXCLUDED.team_id, shirt_number = EXCLUDED.shirt_number, starter = EXCLUDED.starter,
    formation_place = EXCLUDED.formation_place, stats = EXCLUDED.stats;

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

-- name: GetMatchByFlashscoreID :one
SELECT id, flashscore_id::text AS flashscore_id, home_team_id, away_team_id FROM matches WHERE flashscore_id = $1;

-- name: ListPlayersMissingPhoto :many
-- SofaScore players' photos sit at a fixed URL by id; most youth players have none (404),
-- which an empty photo_url records so they are not asked again.
SELECT id,
       coalesce(flashscore_id, 'ss' || sofascore_id)::text AS flashscore_id,
       coalesce(photo_source_url, 'https://img.sofascore.com/api/v1/player/' || sofascore_id || '/image')::text AS photo_source_url
FROM players
WHERE photo_url IS NULL AND (photo_source_url IS NOT NULL OR sofascore_id IS NOT NULL)
LIMIT $1;

-- name: SetPlayerPhoto :exec
UPDATE players SET photo_url = $2 WHERE id = $1;

-- name: SetMatchFormations :exec
UPDATE matches SET home_formation = $2, away_formation = $3 WHERE id = $1;

-- name: ListMatchesToRereadLineups :many
-- Matches whose lineups were stored before formations, slots, and ratings were
-- kept (a starter without a slot), newest first; for ingestor -backfill-lineups.
SELECT m.id, coalesce(m.flashscore_id, '')::text AS flashscore_id, coalesce(m.sofascore_id, 0)::int AS sofascore_id,
       m.home_team_id, m.away_team_id
FROM matches m
WHERE EXISTS (SELECT 1 FROM match_lineups l WHERE l.match_id = m.id AND l.starter AND l.formation_place IS NULL)
ORDER BY m.match_time DESC;
