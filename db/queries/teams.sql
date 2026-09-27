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
SELECT * FROM match_rows
WHERE sqlc.arg(team_id)::uuid IN (home_team_id, away_team_id) AND status <> 'scheduled'
ORDER BY match_time DESC
LIMIT 5;

-- name: ListTeamUpcomingMatches :many
SELECT * FROM match_rows
WHERE sqlc.arg(team_id)::uuid IN (home_team_id, away_team_id) AND status = 'scheduled'
ORDER BY match_time
LIMIT 5;
