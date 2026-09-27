-- Countries own their competitions; a country page is its slug plus its leagues.
CREATE TABLE countries (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name       TEXT NOT NULL UNIQUE,
    sort_order SMALLINT NOT NULL DEFAULT 0
);

INSERT INTO countries (slug, name)
SELECT DISTINCT country_slug, country FROM competitions;

ALTER TABLE competitions ADD COLUMN country_id UUID REFERENCES countries(id);
UPDATE competitions c SET country_id = n.id FROM countries n WHERE n.slug = c.country_slug;
ALTER TABLE competitions
    ALTER COLUMN country_id SET NOT NULL,
    DROP COLUMN country,
    DROP COLUMN country_slug;
CREATE INDEX competitions_country_idx ON competitions (country_id);
