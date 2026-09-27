-- Competitions found on a national team's page (ASEAN Championship, friendlies)
-- hold only that team's matches: the ingestor does not scrape their own page,
-- and the API sends no table for them.
ALTER TABLE competitions ADD COLUMN scraped BOOLEAN NOT NULL DEFAULT true;

-- The ingestor reads the page of every national team whose country has a scraped league.
INSERT INTO teams (flashscore_id, name, short_name, country_id)
SELECT '88ErHiT9', 'Indonesia', 'IND', id FROM countries WHERE slug = 'indonesia'
ON CONFLICT (flashscore_id) DO UPDATE SET country_id = EXCLUDED.country_id;
