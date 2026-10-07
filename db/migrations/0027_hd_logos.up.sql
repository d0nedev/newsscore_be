-- League feeds carry 30x30 team icons; the heading of a club's squad page and
-- of a league page shows the 100x100 logo. Mirroring prefers the larger one.
ALTER TABLE teams ADD COLUMN logo_hd_source_url TEXT;

-- League logos, mirrored like team logos: from the league page heading
-- (Flashscore) or by id (SofaScore). logo_url '' means the source has none.
ALTER TABLE competitions
    ADD COLUMN logo_source_url TEXT,
    ADD COLUMN logo_url        TEXT;
