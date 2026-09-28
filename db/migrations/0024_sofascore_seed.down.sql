DROP TABLE competition_standings;
DELETE FROM players WHERE flashscore_id IS NULL;
DELETE FROM teams WHERE flashscore_id IS NULL;
DELETE FROM competitions WHERE flashscore_path IS NULL;
ALTER TABLE players DROP COLUMN sofascore_id, DROP COLUMN birth_date, DROP COLUMN height_cm, ALTER COLUMN flashscore_id SET NOT NULL;
ALTER TABLE teams DROP COLUMN sofascore_id, ALTER COLUMN flashscore_id SET NOT NULL;
ALTER TABLE competitions DROP COLUMN sofascore_id, ALTER COLUMN flashscore_path SET NOT NULL;
