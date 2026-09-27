-- Push live changes to the API's SSE hub over LISTEN/NOTIFY: the ingestor only
-- writes rows, and every API replica listening on the channel fans them out.
CREATE FUNCTION notify_match_update() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('match_updates', json_build_object(
        'kind', 'score',
        'id', NEW.id,
        'status', NEW.status,
        'stage', NEW.stage,
        'stage_started_at', NEW.stage_started_at,
        'home_score', NEW.home_score,
        'away_score', NEW.away_score
    )::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER matches_notify_update
    AFTER UPDATE ON matches
    FOR EACH ROW
    WHEN ((OLD.status, OLD.stage, OLD.home_score, OLD.away_score)
          IS DISTINCT FROM (NEW.status, NEW.stage, NEW.home_score, NEW.away_score))
    EXECUTE FUNCTION notify_match_update();

-- Only real inserts notify: the ingestor's upserts hit ON CONFLICT DO UPDATE for known events.
CREATE FUNCTION notify_match_event() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('match_updates', json_build_object(
        'kind', 'match_event',
        'id', NEW.match_id,
        'type', NEW.type,
        'minute', NEW.minute,
        'player', NEW.player_name,
        'team', CASE WHEN NEW.team_id IS NULL THEN ''
                     WHEN NEW.team_id = (SELECT home_team_id FROM matches WHERE id = NEW.match_id) THEN 'home'
                     ELSE 'away' END
    )::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER match_events_notify_insert
    AFTER INSERT ON match_events
    FOR EACH ROW
    EXECUTE FUNCTION notify_match_event();
