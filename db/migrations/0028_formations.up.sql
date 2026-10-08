-- Each side's formation ("4-4-2") from the lineups feed. NULL: not read yet
-- (ingestor -backfill-formations fills those); '': the source gives none.
ALTER TABLE matches
    ADD COLUMN home_formation TEXT,
    ADD COLUMN away_formation TEXT;
