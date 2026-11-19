-- Drop Evening Pulse views and tables.
DROP VIEW IF EXISTS pulse_latest_generations;
DROP VIEW IF EXISTS pulse_quality_stats;
DROP VIEW IF EXISTS pulse_syndication_stats;

-- destructive-ok: pulse_selection_log holds derived Evening Pulse output only with no source-of-truth data; the feature is retired, nothing reads or writes it, and the knowledge-sovereign event log is untouched.
DROP TABLE IF EXISTS pulse_selection_log;
-- destructive-ok: pulse_cluster_diagnostics holds derived Evening Pulse output only with no source-of-truth data; the feature is retired, nothing reads or writes it, and the knowledge-sovereign event log is untouched.
DROP TABLE IF EXISTS pulse_cluster_diagnostics;
-- destructive-ok: pulse_generations holds derived Evening Pulse output only with no source-of-truth data; the feature is retired, nothing reads or writes it, and the knowledge-sovereign event log is untouched.
DROP TABLE IF EXISTS pulse_generations;
