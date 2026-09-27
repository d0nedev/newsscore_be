ALTER TABLE competitions ADD COLUMN country TEXT, ADD COLUMN country_slug TEXT CHECK (country_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
UPDATE competitions c SET country = n.name, country_slug = n.slug FROM countries n WHERE n.id = c.country_id;
ALTER TABLE competitions
    ALTER COLUMN country SET NOT NULL,
    ALTER COLUMN country_slug SET NOT NULL,
    DROP COLUMN country_id;
DROP TABLE countries;
