DROP TABLE match_lineups;
ALTER TABLE players DROP COLUMN shirt_number;
DROP INDEX match_events_related_idx;
DROP INDEX match_events_player_idx;
ALTER TABLE match_events
    DROP COLUMN related_flashscore_id,
    DROP COLUMN related_player_name,
    DROP COLUMN player_flashscore_id;
