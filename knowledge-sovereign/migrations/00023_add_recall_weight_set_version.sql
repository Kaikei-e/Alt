-- ADR-000913 §D-9 — Heavy-Ranker explainable scoring.
--
-- weight_set_version pins the weights map the projector used to compute
-- recall_score; older rows default to "v1_fixed" so the column is
-- non-null and the addition is backward compatible.
--
-- score_breakdown is the explainable per-signal contribution row list.
-- Empty array for legacy rows lets clients treat absence as "no
-- breakdown available" without an extra nullability check.
--
-- recall_candidate_view lives in knowledge-sovereign-db since
-- migrations-atlas/20260323100000_drop_sovereign_tables.sql; this
-- migration must run against the sovereign database, not alt-db.

ALTER TABLE recall_candidate_view
  ADD COLUMN IF NOT EXISTS weight_set_version TEXT NOT NULL DEFAULT 'v1_fixed',
  ADD COLUMN IF NOT EXISTS score_breakdown    JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE recall_candidate_view
  ADD CONSTRAINT recall_candidate_weight_set_known CHECK (
    weight_set_version IN ('v1_fixed', 'v2_heavy_ranker')
  );
