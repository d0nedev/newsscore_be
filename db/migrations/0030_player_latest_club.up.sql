-- ingestor -backfill-lineups read lineups newest first and each one set the
-- player's team, so the oldest match won: Rashford ended at Barcelona after
-- playing for Manchester Utd since. UpsertPlayer now ignores older matches;
-- this puts every player whose team came from a lineup back at the club of
-- his latest one (national teams aside). Teams set by a squad page alone are kept.
WITH latest AS (
    SELECT DISTINCT ON (l.player_id) l.player_id, l.team_id, l.shirt_number
    FROM match_lineups l
    JOIN matches m ON m.id = l.match_id
    JOIN teams t ON t.id = l.team_id
    WHERE t.country_id IS NULL
    ORDER BY l.player_id, m.match_time DESC
)
UPDATE players p
SET team_id = latest.team_id,
    shirt_number = coalesce(latest.shirt_number, p.shirt_number)
FROM latest
WHERE latest.player_id = p.id
  AND p.team_id IS DISTINCT FROM latest.team_id
  AND EXISTS (SELECT 1 FROM match_lineups l WHERE l.player_id = p.id AND l.team_id = p.team_id);
