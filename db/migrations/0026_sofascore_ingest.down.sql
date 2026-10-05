DROP TABLE competition_top_players;
ALTER TABLE match_lineups DROP COLUMN stats;
ALTER TABLE match_events DROP COLUMN player_sofascore_id, DROP COLUMN related_sofascore_id;
DELETE FROM ingest_jobs WHERE kind LIKE 'sofascore_%';
DELETE FROM matches WHERE flashscore_id IS NULL;
ALTER TABLE matches DROP COLUMN sofascore_id, ALTER COLUMN flashscore_id SET NOT NULL;
