-- Competitions the ingestor scrapes. Adding a league is one INSERT: the ingestor
-- reads this table on every run.
CREATE TABLE competitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name            TEXT NOT NULL,
    country         TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('league', 'cup')),
    flashscore_path TEXT NOT NULL UNIQUE,  -- e.g. /football/indonesia/super-league/
    active          BOOLEAN NOT NULL DEFAULT true,
    sort_order      SMALLINT NOT NULL DEFAULT 0
);

INSERT INTO competitions (slug, name, country, type, flashscore_path, sort_order) VALUES
    ('super-league',  'Super League',  'Indonesia', 'league', '/football/indonesia/super-league/',  1),
    ('championship',  'Championship',  'Indonesia', 'league', '/football/indonesia/championship/',  2),
    ('president-cup', 'Piala Presiden', 'Indonesia', 'cup',    '/football/indonesia/president-cup/', 3);

-- Every match so far came from the Super League page.
ALTER TABLE matches ADD COLUMN competition_id UUID REFERENCES competitions(id);
UPDATE matches SET competition_id = (SELECT id FROM competitions WHERE slug = 'super-league');
ALTER TABLE matches ALTER COLUMN competition_id SET NOT NULL;
CREATE INDEX matches_competition_season_idx ON matches (competition_id, season);

CREATE OR REPLACE VIEW match_rows AS
SELECT m.id, m.season, m.status, m.match_time, m.home_score, m.away_score, m.stage, m.stage_started_at,
       m.home_team_id, m.away_team_id,
       h.name AS home_name, h.short_name AS home_short_name, h.logo_url AS home_logo_url,
       a.name AS away_name, a.short_name AS away_short_name, a.logo_url AS away_logo_url,
       m.competition_id, c.slug AS competition_slug, c.name AS competition_name, c.sort_order AS competition_sort
FROM matches m
JOIN teams h ON h.id = m.home_team_id
JOIN teams a ON a.id = m.away_team_id
JOIN competitions c ON c.id = m.competition_id;
