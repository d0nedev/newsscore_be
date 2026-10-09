-- SofaScore competitions: the ingestor (migration 0026) and the one-off seed (cmd/seed-sofascore, migration 0024).

-- name: UpsertSofascoreCompetition :one
INSERT INTO competitions (slug, name, country_id, type, sofascore_id, scraped, sort_order)
VALUES ($1, $2, $3, $4, $5, false, 50)
ON CONFLICT (sofascore_id) DO UPDATE SET name = EXCLUDED.name, type = EXCLUDED.type
RETURNING id;

-- name: UpsertSofascoreTeam :one
INSERT INTO teams (sofascore_id, name, short_name, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (sofascore_id) DO UPDATE
SET name = EXCLUDED.name, short_name = EXCLUDED.short_name, updated_at = now()
RETURNING id;

-- name: UpsertSofascorePlayer :one
INSERT INTO players (sofascore_id, team_id, name, nationality, position, birth_date, height_cm, shirt_number, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (sofascore_id) DO UPDATE
SET team_id = EXCLUDED.team_id,
    name = EXCLUDED.name,
    nationality = coalesce(EXCLUDED.nationality, players.nationality),
    position = coalesce(EXCLUDED.position, players.position),
    birth_date = coalesce(EXCLUDED.birth_date, players.birth_date),
    height_cm = coalesce(EXCLUDED.height_cm, players.height_cm),
    shirt_number = coalesce(EXCLUDED.shirt_number, players.shirt_number),
    updated_at = now()
RETURNING id;

-- name: DeleteStandingSnapshot :exec
DELETE FROM competition_standings WHERE competition_id = $1 AND season = $2;

-- name: InsertStandingSnapshot :exec
INSERT INTO competition_standings (competition_id, season, group_name, group_order, position, team_id,
                                   played, won, drawn, lost, goals_for, goals_against, points)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ListStandingSnapshot :many
SELECT s.group_name, s.position, t.id, t.name, t.short_name, t.logo_url,
       s.played, s.won, s.drawn, s.lost, s.goals_for, s.goals_against, s.points
FROM competition_standings s
JOIN teams t ON t.id = s.team_id
WHERE s.competition_id = $1 AND s.season = $2
ORDER BY s.group_order, s.position;

-- name: ListSofascoreCompetitions :many
SELECT id, slug, name, sofascore_id::int AS sofascore_id
FROM competitions
WHERE active AND sofascore_id IS NOT NULL AND flashscore_path IS NULL
ORDER BY sort_order, slug;

-- name: UpsertSofascoreMatch :exec
INSERT INTO matches (sofascore_id, competition_id, season, home_team_id, away_team_id, status, match_time,
                     home_score, away_score, round, data_as_of, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sofascore_id) DO UPDATE
SET status = EXCLUDED.status,
    season = EXCLUDED.season,
    match_time = EXCLUDED.match_time,
    home_score = EXCLUDED.home_score,
    away_score = EXCLUDED.away_score,
    round = EXCLUDED.round,
    data_as_of = now(),
    updated_at = now();

-- name: GetMatchBySofascoreID :one
SELECT id, home_team_id, away_team_id FROM matches WHERE sofascore_id = $1;

-- name: EnqueueSofascoreMatches :execrows
-- Finished SofaScore matches without details, newest first; see EnqueueMatchDetails.
INSERT INTO ingest_jobs (kind, target)
SELECT 'sofascore_match', m.sofascore_id::text
FROM matches m
WHERE m.status = 'finished'
  AND m.sofascore_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM match_statistics s WHERE s.match_id = m.id)
ORDER BY m.match_time DESC
ON CONFLICT (kind, target) DO UPDATE
SET status = 'pending', attempts = 0, not_before = now(), updated_at = now()
WHERE ingest_jobs.status = 'done';

-- name: UpsertSofascoreMatchEvent :exec
INSERT INTO match_events (match_id, flashscore_id, type, minute, player_name, team_id,
                          player_sofascore_id, related_player_name, related_sofascore_id, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (match_id, flashscore_id) DO UPDATE
SET type = EXCLUDED.type,
    minute = EXCLUDED.minute,
    player_name = EXCLUDED.player_name,
    team_id = EXCLUDED.team_id,
    player_sofascore_id = EXCLUDED.player_sofascore_id,
    related_player_name = EXCLUDED.related_player_name,
    related_sofascore_id = EXCLUDED.related_sofascore_id,
    updated_at = now();

-- name: UpsertSofascoreLineup :exec
INSERT INTO match_lineups (match_id, player_id, team_id, shirt_number, starter, stats)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (match_id, player_id) DO UPDATE
SET team_id = EXCLUDED.team_id, shirt_number = EXCLUDED.shirt_number, starter = EXCLUDED.starter, stats = EXCLUDED.stats;

-- name: ListSofascoreSquadsToSync :many
-- Teams in a SofaScore competition's table whose squad was not read this week, oldest first.
SELECT t.id, t.sofascore_id::int AS sofascore_id
FROM teams t
WHERE t.sofascore_id IS NOT NULL
  AND (t.squad_synced_at IS NULL OR t.squad_synced_at < now() - interval '7 days')
  AND EXISTS (SELECT 1 FROM competition_standings s WHERE s.team_id = t.id)
ORDER BY t.squad_synced_at NULLS FIRST
LIMIT $1;

-- name: DeleteTopPlayers :exec
DELETE FROM competition_top_players WHERE competition_id = $1 AND season = $2;

-- name: InsertTopPlayer :exec
INSERT INTO competition_top_players (competition_id, season, category, rank, player_id, team_id, value, appearances)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListSofascoreMatchesWithoutDetails :many
-- For cmd/seed-sofascore: the backlog EnqueueSofascoreMatches would queue, fetched in one go.
SELECT m.sofascore_id::int AS sofascore_id
FROM matches m
WHERE m.status = 'finished'
  AND m.sofascore_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM match_statistics s WHERE s.match_id = m.id)
ORDER BY m.match_time DESC;

-- name: ListTopPlayers :many
-- A season's leaderboards as SofaScore ranks them, one row per category and rank.
SELECT tp.category, tp.rank, tp.value::float8 AS value, tp.appearances,
       p.id AS player_id, p.name AS player_name, p.photo_url,
       t.id AS team_id, t.name AS team_name, t.short_name AS team_short_name, t.logo_url AS team_logo_url
FROM competition_top_players tp
JOIN players p ON p.id = tp.player_id
LEFT JOIN teams t ON t.id = tp.team_id
WHERE tp.competition_id = $1 AND tp.season = $2
ORDER BY tp.category, tp.rank;
