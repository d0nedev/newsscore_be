-- The squad page (/team/<slug>/<id>/squad/) needs the team's URL slug; the
-- ingestor reads each club's squad about once a day for player positions.
ALTER TABLE teams
    ADD COLUMN flashscore_slug  TEXT,
    ADD COLUMN squad_synced_at  TIMESTAMPTZ;
UPDATE teams t SET flashscore_slug = n.slug FROM countries n WHERE n.id = t.country_id;
