DELETE FROM matches WHERE competition_id IN (SELECT id FROM competitions WHERE sort_order >= 10 AND scraped);
DELETE FROM competitions WHERE sort_order >= 10 AND scraped;
DELETE FROM countries n WHERE slug IN ('england', 'spain', 'italy', 'germany', 'france', 'netherlands', 'portugal', 'turkey', 'europe', 'south-korea', 'usa', 'brazil')
  AND NOT EXISTS (SELECT 1 FROM competitions c WHERE c.country_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM teams t WHERE t.country_id = n.id);
ALTER TABLE countries DROP COLUMN scrape_national_team;
