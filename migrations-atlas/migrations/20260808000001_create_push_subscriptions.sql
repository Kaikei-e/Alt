-- Web Push subscriptions: one row per device, not per user. The same person
-- subscribing from a phone and a desktop produces two endpoints and both
-- receive, which is why the browser's endpoint is the primary key.
--
-- This is resource state, not an event log: the browser hands out the same
-- endpoint again after a permission re-prompt, so re-registering replaces the
-- key material in place. Rows leave only by DeletePushSubscription or by the
-- dispatcher observing 404/410 from the push service — the only signal a
-- device has gone away.
--
-- `id` is a surrogate that nothing outside this database dereferences. It
-- exists so push_deliveries can reference a device without carrying the
-- capability URL into a second table, its indexes, and every row of the
-- delivery queue's history.
CREATE TABLE push_subscriptions (
  id                    UUID        NOT NULL DEFAULT gen_random_uuid(),
  user_id               UUID        NOT NULL,
  endpoint              TEXT        NOT NULL,
  p256dh                TEXT        NOT NULL,
  auth                  TEXT        NOT NULL,
  summary_ready         BOOLEAN     NOT NULL DEFAULT TRUE,
  acolyte_report_ready  BOOLEAN     NOT NULL DEFAULT TRUE,
  recap_ready           BOOLEAN     NOT NULL DEFAULT TRUE,
  today_entrance_ready  BOOLEAN     NOT NULL DEFAULT TRUE,
  vapid_key_fingerprint TEXT        NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  last_success_at       TIMESTAMPTZ,
  last_failure_at       TIMESTAMPTZ,
  PRIMARY KEY (endpoint),
  UNIQUE (id)
);

-- Fan-out reads every device of one user.
CREATE INDEX idx_push_subscriptions_user
  ON push_subscriptions (user_id);

COMMENT ON TABLE push_subscriptions IS 'Web Push subscriptions, one row per device (endpoint is the browser''s own handle)';

COMMENT ON COLUMN push_subscriptions.endpoint IS
  'Capability URL: anyone holding it can push to this device. Never log it, never return it in an error message, never include it in a trace attribute.';

COMMENT ON COLUMN push_subscriptions.id IS
  'Surrogate device id, referenced by push_deliveries so the delivery queue never carries the capability URL.';

COMMENT ON COLUMN push_subscriptions.vapid_key_fingerprint IS
  'Which VAPID keypair this subscription was created under. Rotating the keypair invalidates every existing subscription and there is no server-side migration for that, so the row has to record it: the dispatcher skips rows created under a retired key and the client re-subscribes after comparing GetPushConfig against its own.';
