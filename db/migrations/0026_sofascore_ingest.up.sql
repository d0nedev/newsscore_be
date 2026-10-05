-- The ingestor reads SofaScore's API for competitions that have a sofascore_id
-- and no Flashscore path (Liga Nusantara, EPA, Liga Topskor): matches, details,
-- squads, tables, and top players. Their rows carry SofaScore ids, not Flashscore ones.
ALTER TABLE matches
    ALTER COLUMN flashscore_id DROP NOT NULL,
    ADD COLUMN sofascore_id INT UNIQUE;

-- match_events.flashscore_id is the source's incident id; SofaScore incidents
-- name players by SofaScore id.
ALTER TABLE match_events
    ADD COLUMN player_sofascore_id  INT,
    ADD COLUMN related_sofascore_id INT;
CREATE INDEX match_events_player_ss_idx  ON match_events (player_sofascore_id)  WHERE player_sofascore_id IS NOT NULL;
CREATE INDEX match_events_related_ss_idx ON match_events (related_sofascore_id) WHERE related_sofascore_id IS NOT NULL;

-- A player's numbers in one match (rating, passes, ...), as SofaScore sends them.
ALTER TABLE match_lineups ADD COLUMN stats JSONB;

-- Leaderboards as SofaScore ranks them (goals, assists, rating, ...).
CREATE TABLE competition_top_players (
    competition_id UUID NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
    season         SMALLINT NOT NULL,
    category       TEXT NOT NULL,  -- SofaScore's key: goals, assists, redCards, rating, ...
    rank           SMALLINT NOT NULL,
    player_id      UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    team_id        UUID REFERENCES teams(id),
    value          NUMERIC NOT NULL,
    appearances    SMALLINT,
    PRIMARY KEY (competition_id, season, category, rank)
);
