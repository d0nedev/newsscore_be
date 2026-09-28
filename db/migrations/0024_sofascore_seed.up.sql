-- One-off seed of Indonesian competitions Flashscore lacks (Liga Nusantara,
-- EPA, Liga Topskor) from SofaScore pages saved by scrap-claude/sofascore_spike.py.
-- Those rows have a sofascore_id and no Flashscore id or path.
ALTER TABLE competitions
    ALTER COLUMN flashscore_path DROP NOT NULL,
    ADD COLUMN sofascore_id INT UNIQUE;
ALTER TABLE teams
    ALTER COLUMN flashscore_id DROP NOT NULL,
    ADD COLUMN sofascore_id INT UNIQUE;
ALTER TABLE players
    ALTER COLUMN flashscore_id DROP NOT NULL,
    ADD COLUMN sofascore_id INT UNIQUE,
    ADD COLUMN birth_date DATE,
    ADD COLUMN height_cm SMALLINT;

-- SofaScore gives these competitions' tables but not their matches, so the
-- table is stored as seen instead of computed from matches.
CREATE TABLE competition_standings (
    competition_id UUID NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    season         SMALLINT NOT NULL,
    group_name     TEXT NOT NULL DEFAULT '',  -- '' for a single table, else "Group A" or "Second Round, Group A"
    group_order    SMALLINT NOT NULL DEFAULT 0, -- the source's table order
    position       SMALLINT NOT NULL,
    team_id        UUID NOT NULL REFERENCES teams(id),
    played         SMALLINT NOT NULL,
    won            SMALLINT NOT NULL,
    drawn          SMALLINT NOT NULL,
    lost           SMALLINT NOT NULL,
    goals_for      SMALLINT NOT NULL,
    goals_against  SMALLINT NOT NULL,
    points         SMALLINT NOT NULL,
    -- a team plays several stages: first round, second round, 3rd place
    PRIMARY KEY (competition_id, season, group_name, team_id)
);
