-- Add `internalized` to the dismiss_state enum.
--
-- ADR-000908 §Δ3: terminal "knowledge internalized" graduation state. The
-- entry has been understood and graduated out of the Loop's read model. The
-- event log retains the transition so reproject can rebuild this row, and
-- the read path filters `dismiss_state='internalized'` rows from foreground
-- and bucket results. macroState.recentInternalizedCount (ADR-000909) is the
-- 7-day count that surfaces the "卒業" feedback on /loop.
--
-- Pure enum extension — no row migration, no CHECK constraint impact, no
-- existing column rename. Existing rows keep their current dismiss_state;
-- new transitions explicitly set 'internalized' via the projector.

ALTER TYPE dismiss_state ADD VALUE IF NOT EXISTS 'internalized';
