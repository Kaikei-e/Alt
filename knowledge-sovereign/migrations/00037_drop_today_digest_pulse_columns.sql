-- atlas:nolint destructive
--
-- Drop Evening Pulse columns from today_digest_view.
-- today_digest_view is a disposable CQRS projection rebuildable from
-- append-only knowledge_events; Evening Pulse is retired and nothing reads
-- or writes these columns. Historical recap.evening_pulse_generated.v1 rows
-- remain untouched in knowledge_events.

-- destructive-ok: today_digest_view is a disposable projection; evening_pulse_available is retired, nothing reads or writes it, and knowledge_events retains all historical events.
ALTER TABLE today_digest_view DROP COLUMN IF EXISTS evening_pulse_available;

-- destructive-ok: today_digest_view is a disposable projection; pulse_refs_json is retired, nothing reads or writes it, and knowledge_events retains all historical events.
ALTER TABLE today_digest_view DROP COLUMN IF EXISTS pulse_refs_json;
