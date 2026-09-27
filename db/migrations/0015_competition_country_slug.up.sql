-- Clients link country pages by slug; sending it saves every client its own slugify.
ALTER TABLE competitions ADD COLUMN country_slug TEXT CHECK (country_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
UPDATE competitions SET country_slug = lower(replace(country, ' ', '-'));
ALTER TABLE competitions ALTER COLUMN country_slug SET NOT NULL;
