-- Team logos are served from our own /assets instead of hotlinking Flashscore.
-- logo_source_url is where the ingestor downloads from; logo_url is the local
-- path clients get, NULL until the file is stored.
ALTER TABLE teams ADD COLUMN logo_source_url TEXT;
UPDATE teams SET logo_source_url = logo_url, logo_url = NULL;
