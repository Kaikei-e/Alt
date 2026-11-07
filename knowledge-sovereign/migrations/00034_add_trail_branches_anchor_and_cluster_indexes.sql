-- Add indexes on knowledge_trail_branches for page-scoped anchor lookups in
-- GetLatestOpenTrailBranchesForAnchors and the cluster-candidate anti-join in
-- DeriveTrailClusterCandidates.

CREATE INDEX IF NOT EXISTS idx_trail_branches_user_anchor_open
  ON knowledge_trail_branches (user_id, anchor_item_key, created_at DESC, branch_key DESC)
  WHERE state = 'open';

CREATE INDEX IF NOT EXISTS idx_trail_branches_user_target_cluster
  ON knowledge_trail_branches (user_id, target_item_key)
  WHERE relation_kind = 'cluster';
