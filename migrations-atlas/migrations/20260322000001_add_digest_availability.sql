-- Add digest availability flags (backend-authoritative)
ALTER TABLE today_digest_view
  ADD COLUMN weekly_recap_available BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN evening_pulse_available BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN today_digest_view.weekly_recap_available IS 'Backend-authoritative flag for weekly recap CTA';
COMMENT ON COLUMN today_digest_view.evening_pulse_available IS 'Backend-authoritative flag for evening pulse CTA';
