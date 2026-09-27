-- A team with a country is that country's national team; NULL is a club.
ALTER TABLE teams ADD COLUMN country_id UUID REFERENCES countries(id);
CREATE UNIQUE INDEX teams_country_idx ON teams (country_id);
