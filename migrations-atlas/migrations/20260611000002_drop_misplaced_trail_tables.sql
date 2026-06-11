-- atlas:nolint destructive
--
-- Reconcile alt_db with schema.hcl. The knowledge_trail_footprints /
-- knowledge_trail_branches tables were created here by mistake — they are
-- Knowledge Sovereign read models and belong to the knowledge_sovereign DB
-- (now created by knowledge-sovereign/migrations/00026 + 00027). They were never
-- part of alt_db's schema.hcl, so the earlier CREATE migrations left this
-- directory's replay state diverged from the declared schema.
--
-- These tables hold no alt_db data (nothing in alt_db reads or writes them); the
-- IF EXISTS drop is a no-op when the earlier CREATEs were never applied.
DROP TABLE IF EXISTS knowledge_trail_branches;
DROP TABLE IF EXISTS knowledge_trail_footprints;
