DELETE FROM matches WHERE competition_id IN (SELECT id FROM competitions WHERE NOT scraped);
DELETE FROM competitions WHERE NOT scraped;
ALTER TABLE competitions DROP COLUMN scraped;
UPDATE teams SET country_id = NULL;
