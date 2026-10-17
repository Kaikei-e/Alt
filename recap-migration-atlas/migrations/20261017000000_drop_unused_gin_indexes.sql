-- Drop unused GIN indexes (idx_scan = 0) to reclaim disk space and reduce write amplification
DROP INDEX IF EXISTS idx_recap_genre_learning_results_coarse_candidates_gin;
DROP INDEX IF EXISTS idx_recap_subworker_runs_response_payload_gin;
