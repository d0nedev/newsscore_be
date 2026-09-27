-- name: GetPlayer :one
SELECT p.id, p.flashscore_id, p.name, p.shirt_number, p.position, p.nationality,
       t.id AS team_id, t.name AS team_name, t.short_name AS team_short_name, t.logo_url AS team_logo_url
FROM players p
LEFT JOIN teams t ON t.id = p.team_id
WHERE p.id = $1;

-- name: ListPlayerMatches :many
-- Every match the player was listed for in a season (optionally one competition), newest first, with what they did in it.
SELECT m.id, m.match_time, m.status, m.home_score, m.away_score, m.round,
       c.slug AS competition_slug, c.name AS competition_name,
       l.starter, l.team_id,
       h.name AS home_name, a.name AS away_name,
       m.home_team_id,
       EXISTS (SELECT 1 FROM match_events e WHERE e.match_id = m.id AND e.type = 'Substitution'
                 AND e.player_flashscore_id = sqlc.arg(flashscore_id)::text) AS subbed_on,
       (SELECT count(*) FROM match_events e WHERE e.match_id = m.id AND e.type IN ('Goal', 'Penalty')
          AND e.player_flashscore_id = sqlc.arg(flashscore_id)::text)::int AS goals,
       (SELECT count(*) FROM match_events e WHERE e.match_id = m.id AND e.type IN ('Goal', 'Penalty')
          AND e.related_flashscore_id = sqlc.arg(flashscore_id)::text)::int AS assists,
       (SELECT count(*) FROM match_events e WHERE e.match_id = m.id AND e.type = 'Yellow Card'
          AND e.player_flashscore_id = sqlc.arg(flashscore_id)::text)::int AS yellow,
       (SELECT count(*) FROM match_events e WHERE e.match_id = m.id AND e.type IN ('Red Card', 'Yellow/Red Card')
          AND e.player_flashscore_id = sqlc.arg(flashscore_id)::text)::int AS red
FROM match_lineups l
JOIN matches m ON m.id = l.match_id
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
JOIN competitions c ON c.id = m.competition_id
WHERE l.player_id = sqlc.arg(player_id)::uuid AND m.season = sqlc.arg(season)::smallint
  AND (sqlc.narg(competition_id)::uuid IS NULL OR m.competition_id = sqlc.narg(competition_id)::uuid)
ORDER BY m.match_time DESC;
