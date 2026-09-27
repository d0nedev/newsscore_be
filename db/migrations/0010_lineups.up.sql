-- Who played and who scored, so team squads and player pages can count
-- appearances, goals, and assists. Player links use Flashscore ids because
-- incidents can name a player before their lineup row is stored.
ALTER TABLE match_events
    ADD COLUMN player_flashscore_id  TEXT,
    ADD COLUMN related_player_name   TEXT,
    ADD COLUMN related_flashscore_id TEXT;
CREATE INDEX match_events_player_idx  ON match_events (player_flashscore_id)  WHERE player_flashscore_id IS NOT NULL;
CREATE INDEX match_events_related_idx ON match_events (related_flashscore_id) WHERE related_flashscore_id IS NOT NULL;

ALTER TABLE players
    ADD COLUMN shirt_number SMALLINT;

CREATE TABLE match_lineups (
    match_id     UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    player_id    UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    team_id      UUID NOT NULL REFERENCES teams(id),
    shirt_number SMALLINT,
    starter      BOOLEAN NOT NULL,
    PRIMARY KEY (match_id, player_id)
);
CREATE INDEX match_lineups_player_idx ON match_lineups (player_id);

-- Earlier ingests kept only the last part of multi-part incidents (a goal was
-- stored as its assist). Clearing stats makes the ingestor fetch details again.
DELETE FROM match_statistics;
