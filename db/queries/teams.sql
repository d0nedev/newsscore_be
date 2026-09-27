-- name: GetTeam :one
SELECT id, name, short_name, logo_url FROM teams WHERE id = $1;

-- name: ListTeamSquad :many
-- Current squad with season numbers. An appearance is a start or a substitution on.
SELECT p.id, p.name, p.shirt_number, p.position, p.nationality,
       (SELECT count(*) FROM match_lineups l JOIN matches m ON m.id = l.match_id
         WHERE l.player_id = p.id AND m.season = sqlc.arg(season)::smallint
           AND (l.starter OR EXISTS (SELECT 1 FROM match_events e
                                      WHERE e.match_id = l.match_id AND e.type = 'Substitution'
                                        AND e.player_flashscore_id = p.flashscore_id)))::int AS apps,
       (SELECT count(*) FROM match_events e JOIN matches m ON m.id = e.match_id
         WHERE e.player_flashscore_id = p.flashscore_id AND e.type IN ('Goal', 'Penalty')
           AND m.season = sqlc.arg(season)::smallint)::int AS goals,
       (SELECT count(*) FROM match_events e JOIN matches m ON m.id = e.match_id
         WHERE e.related_flashscore_id = p.flashscore_id AND e.type IN ('Goal', 'Penalty')
           AND m.season = sqlc.arg(season)::smallint)::int AS assists
FROM players p
WHERE p.team_id = sqlc.arg(team_id)::uuid
ORDER BY p.position = 'GK' DESC, p.shirt_number NULLS LAST, p.name;

-- name: ListTeamRecentMatches :many
SELECT m.id, m.status, m.match_time, m.home_score, m.away_score, m.stage, m.stage_started_at,
       h.id AS home_id, h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.id AS away_id, a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
WHERE $1 IN (m.home_team_id, m.away_team_id) AND m.status <> 'scheduled'
ORDER BY m.match_time DESC
LIMIT 5;

-- name: ListTeamUpcomingMatches :many
SELECT m.id, m.status, m.match_time, m.home_score, m.away_score, m.stage, m.stage_started_at,
       h.id AS home_id, h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.id AS away_id, a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
WHERE $1 IN (m.home_team_id, m.away_team_id) AND m.status = 'scheduled'
ORDER BY m.match_time
LIMIT 5;
