UPDATE teams SET logo_url = logo_source_url;
ALTER TABLE teams DROP COLUMN logo_source_url;
