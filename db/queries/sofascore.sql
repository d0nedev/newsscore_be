-- One-off seed from SofaScore (cmd/seed-sofascore); see migration 0024.

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

-- name: UpsertSofascorePlayer :exec
INSERT INTO players (sofascore_id, team_id, name, nationality, position, birth_date, height_cm, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (sofascore_id) DO UPDATE
SET team_id = EXCLUDED.team_id,
    name = EXCLUDED.name,
    nationality = coalesce(EXCLUDED.nationality, players.nationality),
    position = coalesce(EXCLUDED.position, players.position),
    birth_date = coalesce(EXCLUDED.birth_date, players.birth_date),
    height_cm = coalesce(EXCLUDED.height_cm, players.height_cm),
    updated_at = now();

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
