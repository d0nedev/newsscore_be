-- Fuzzy name search: q <% name matches when q is similar to some word run in name
-- (typos like "persb" still find "Persib Bandung"); the GIN indexes serve that operator.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX teams_name_trgm_idx   ON teams   USING gin (name gin_trgm_ops);
CREATE INDEX players_name_trgm_idx ON players USING gin (name gin_trgm_ops);
CREATE INDEX news_title_trgm_idx   ON news    USING gin (title gin_trgm_ops);
