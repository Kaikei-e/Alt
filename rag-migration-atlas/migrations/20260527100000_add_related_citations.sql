-- Add related_citations JSONB column to augur_messages.
--
-- The column captures an inline-projected snapshot of articles semantically
-- and lexically near the direct citations at the moment an assistant turn is
-- materialized. Append-only invariant is preserved: the column is written on
-- the same INSERT as citations and is never updated afterwards. Legacy rows
-- read back with the default empty array, which the UI renders as no
-- "Related" section.
ALTER TABLE augur_messages
    ADD COLUMN related_citations JSONB NOT NULL DEFAULT '[]'::jsonb;
