-- Trigger to notify projectors when new knowledge events are inserted.
-- Used by WatchProjectorEvents streaming RPC to push notifications to alt-backend.

CREATE OR REPLACE FUNCTION notify_knowledge_projector() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('knowledge_projector', NEW.event_seq::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_knowledge_events_notify
  AFTER INSERT ON knowledge_events
  FOR EACH ROW
  EXECUTE FUNCTION notify_knowledge_projector();
