package sovereign_db

// activeProjectionVersionSQL is the shared subquery that resolves the currently
// active projection version (falling back to 1 when none is marked active).
const activeProjectionVersionSQL = `COALESCE((
	SELECT version FROM knowledge_projection_versions
	WHERE status = 'active'
	ORDER BY version DESC LIMIT 1
), 1)`

// homeItemRankScoreSQL ranks a knowledge_home_items row by its stored,
// time-invariant quality/affinity score (khi.score — see baseQualityScore in
// knowledge_home_projector) decayed by how long ago it was published. This is
// deliberately NOT a stored column: baking a (some-fixed-instant -
// published_at) decay into the projector's write path was the
// frozen-ranking bug — a score snapshotted once at ArticleCreated fold time,
// then merge-safe-UPSERTed with GREATEST, could never reflect the item's
// actual growing staleness. Falls back to generated_at when published_at is
// unknown (e.g. superseded rows that never carried a publish date).
//
// asOfExpr is the SQL text for "now" — either the literal `now()` (first
// page of a pagination session) or a bound `$N::timestamptz` parameter
// carrying the exact instant a previous page anchored on (continuation
// pages). Decay strictly shrinks as time passes, so if a continuation page
// re-evaluated `now()` instead of reusing the session's anchor, the
// boundary row's rank_score would be strictly lower than the value the
// cursor captured, satisfy the keyset "<" predicate against itself, and be
// re-emitted forever. See GetKnowledgeHomeItems for how the anchor is
// captured and threaded through.
//
// Every literal here is a bare integer (86400, 1, 0), never a
// decimal-point numeric literal — Postgres types `86400.0` as `numeric`,
// and `numeric / double precision` is not a well-defined operator, whereas
// `int4 / double precision` resolves via the standard implicit int4->float8
// cast. khi.score (NUMERIC column) is explicitly cast once up front so the
// whole expression stays in double precision throughout.
func homeItemRankScoreSQL(asOfExpr string) string {
	return `khi.score::double precision / (1 + GREATEST(EXTRACT(EPOCH FROM (` + asOfExpr + ` - COALESCE(khi.published_at, khi.generated_at))) / 86400, 0))`
}
