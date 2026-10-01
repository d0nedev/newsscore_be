-- A league page lists at most 100 fixtures and 100 results; each club's page
-- (team_page jobs, weekly) fills in the rest. Those pages carry no season, so
-- a match's season comes from its date: split_season (from the league page
-- heading, "2026/2027") puts January to June in the season started the year before.
ALTER TABLE competitions ADD COLUMN split_season BOOLEAN NOT NULL DEFAULT false;
