-- Make morning_letters append-only across generation revisions per edition date and timezone
DROP INDEX IF EXISTS idx_morning_letters_target_date;

CREATE UNIQUE INDEX idx_morning_letters_target_date_tz_revision
    ON morning_letters (target_date, edition_timezone, generation_revision);

CREATE INDEX idx_morning_letters_latest_revision
    ON morning_letters (target_date DESC, generation_revision DESC);
