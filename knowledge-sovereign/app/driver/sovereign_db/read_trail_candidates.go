package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// continuationStaleAfter is the minimum quiet period before an engaged-but-
// not-deep thread qualifies for a Continuation candidate (D27/D28, Wave 11):
// a thread gone quiet, not one the user is still actively reading.
const continuationStaleAfter = 3 * 24 * time.Hour

// continuationExpireAfter is the outer bound past which a quiet thread reads
// as gone cold rather than merely quiet, and Continuation stops proposing it.
const continuationExpireAfter = 21 * 24 * time.Hour

// JunkTagWords is the canonical lowercase junk-tag vocabulary: English filler
// words, high-document-frequency generic tech/news meta-tags, Japanese function
// words and generic meta-tags, and URL/HTML debris. Exported so usecase/tagclean
// shares the same vocabulary without a driver->usecase import.
var JunkTagWords = []string{
	// English function/filler words seen verbatim in production tag sets.
	"also", "could", "might", "would", "said", "says",
	"wrote", "becomes", "without", "even", "great",
	"three", "week", "types", "example",
	// High-document-frequency generic tech/news meta-tags that cause Union-Find
	// megaclusters across unrelated articles.
	"ai", "llm", "api", "code", "data", "user", "users",
	"model", "models", "system", "systems", "tool", "tools",
	"app", "apps", "web", "blog", "news", "tech", "software",
	// Japanese function words & generic meta-tags.
	"こと", "もの", "ため", "よう", "それ", "これ",
	"情報", "技術", "開発", "設計", "実装", "処理", "構造",
	"利用", "機能", "記事", "解説", "紹介", "方法", "対応",
	"更新", "問題", "結果", "サービス", "システム", "データ",
	"ユーザー", "ツール", "モデル", "コード", "アプリ",
	"エンジニア", "プログラマー",
	// URL / HTML debris.
	"https", "http", "www", "com", "gt", "lt", "amp",
}

// TrailContinuationCandidate is a thread the user already engaged (self-
// referential) that has gone quiet without going deep: the raw material for a
// Continuation branch ("pick this thread back up").
type TrailContinuationCandidate struct {
	TenantID      uuid.UUID
	TargetItemKey string
	TargetTitle   string
	LastContactAt time.Time
	// Verb is the engagement verb of the latest qualifying contact, so the
	// branch's why names the act that actually happened.
	Verb string
}

func calcTrailContinuationCutoffs(reference time.Time) (time.Time, time.Time) {
	return reference.Add(-continuationStaleAfter), reference.Add(-continuationExpireAfter)
}

func scanTrailContinuationCandidateRow(scanner rowScanner) (TrailContinuationCandidate, error) {
	var c TrailContinuationCandidate
	if err := scanner.Scan(&c.TenantID, &c.TargetItemKey, &c.TargetTitle, &c.LastContactAt, &c.Verb); err != nil {
		return TrailContinuationCandidate{}, err
	}
	return c, nil
}

// DeriveTrailContinuationCandidates finds a thread the user already engaged
// that has gone quiet without going deep — self-referential raw material for a
// Continuation branch: the target IS the anchor, because past engagement with
// the same item is what qualifies it.
//
// Contact means engagement (EngagementVerbs) — a dismissal is the opposite of
// wanting a thread back, so it neither counts as a contact nor sets the quiet
// clock, and an item the user dismissed from Home is never proposed at all.
//
// "Not deep" mirrors the wear CASE in GetTrailFootprints: 1-3 raw contacts, no
// 'asked' verb, no engaged act-outcome. The last contact must sit strictly
// between continuationExpireAfter and continuationStaleAfter ago — a thread
// gone quiet, not one still being read or one gone cold. Items that already
// carry a continuation branch (open or resolved) are excluded so a taken or
// dismissed proposal is never re-proposed. Ordered most-recent-contact first;
// producer-side derivation (the planner reads current state to decide what to
// emit — the projector folding the resulting event stays payload-only).
func (r *Repository) DeriveTrailContinuationCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]TrailContinuationCandidate, error) {
	now := time.Now()
	staleCutoff, expireCutoff := calcTrailContinuationCutoffs(now)

	q := `
WITH active_version AS (
  SELECT ` + activeProjectionVersionSQL + ` AS v
),
item_contacts AS (
  SELECT item_key,
         count(*) AS contact_count,
         bool_or(verb = 'asked') AS has_ask,
         max(occurred_at) AS last_contact_at,
         (array_agg(verb ORDER BY occurred_at DESC, footprint_key DESC))[1] AS last_verb
  FROM knowledge_trail_footprints
  WHERE user_id = $1
    AND verb = ANY($2::text[])
  GROUP BY item_key
),
item_engagement AS (
  SELECT item_key, TRUE AS engaged
  FROM knowledge_trail_act_outcomes
  WHERE user_id = $1
    AND ((dwell_ms IS NOT NULL AND dwell_ms >= $3)
         OR legacy_outcome IN ('engaged', 'deep_engagement'))
  GROUP BY item_key
)
SELECT khi.tenant_id, ic.item_key, khi.title, ic.last_contact_at, ic.last_verb
FROM item_contacts ic
JOIN knowledge_home_items khi
  ON khi.user_id = $1
 AND khi.item_key = ic.item_key
 AND khi.projection_version = (SELECT v FROM active_version)
 AND khi.dismissed_at IS NULL
LEFT JOIN item_engagement ie ON ie.item_key = ic.item_key
WHERE ic.contact_count BETWEEN 1 AND 3
  AND NOT ic.has_ask
  AND NOT COALESCE(ie.engaged, FALSE)
  AND ic.last_contact_at <= $4
  AND ic.last_contact_at >= $5
  AND coalesce(khi.title, '') <> ''
  AND NOT EXISTS (
    SELECT 1 FROM knowledge_trail_branches btb
    WHERE btb.user_id = $1
      AND btb.relation_kind = 'continuation'
      AND btb.target_item_key = ic.item_key
  )
ORDER BY ic.last_contact_at DESC
LIMIT $6`
	rows, err := r.pool.Query(ctx, q, userID, EngagementVerbs, EngagedDwellMs, staleCutoff, expireCutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("DeriveTrailContinuationCandidates: %w", err)
	}
	defer rows.Close()

	var out []TrailContinuationCandidate
	for rows.Next() {
		c, err := scanTrailContinuationCandidateRow(rows)
		if err != nil {
			return nil, fmt.Errorf("DeriveTrailContinuationCandidates scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
