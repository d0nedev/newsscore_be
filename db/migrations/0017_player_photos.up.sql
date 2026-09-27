-- Player photos, stored like team logos: source URL from the lineup feed,
-- local /assets path once the ingestor has downloaded it.
ALTER TABLE players
    ADD COLUMN photo_source_url TEXT,
    ADD COLUMN photo_url        TEXT;

-- Photo sources come from lineups, which are only read with match details;
-- clearing stats makes the ingestor fetch details (and lineups) again.
DELETE FROM match_statistics;
