DROP VIEW match_rows;
CREATE VIEW match_rows AS
SELECT m.id, m.season, m.status, m.match_time, m.home_score, m.away_score, m.stage, m.stage_started_at,
       m.home_team_id, m.away_team_id,
       h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url,
       m.competition_id, c.slug AS competition_slug, c.name AS competition_name, c.sort_order AS competition_sort
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
JOIN competitions c ON c.id = m.competition_id;

ALTER TABLE matches DROP COLUMN phase, DROP COLUMN round;
