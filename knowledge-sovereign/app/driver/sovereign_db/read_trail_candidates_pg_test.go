//go:build integration

package sovereign_db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

func TestDeriveTrailClusterCandidates_RanksOnCleanTagsAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	userID := uuid.New()
	tenantID := uuid.New()

	baseAt := time.Now().UTC().Add(-1 * time.Hour)
	t0 := baseAt.Add(-10 * time.Minute)
	t1 := baseAt
	t2 := baseAt.Add(10 * time.Minute)
	t3 := baseAt.Add(20 * time.Minute)

	seedHomeItem := func(itemKey, title, tagsJSON string, at time.Time) {
		pgExec(ctx, t, db, `INSERT INTO knowledge_home_items
			(user_id, tenant_id, item_key, item_type, title, tags_json, generated_at, updated_at)
			VALUES ($1, $2, $3, 'article', $4, $5::jsonb, $6, $6)`,
			userID, tenantID, itemKey, title, tagsJSON, at)
	}
	seedFootprint := func(fpKey, verb, itemKey string, at time.Time) {
		pgExec(ctx, t, db, `INSERT INTO knowledge_trail_footprints
			(user_id, tenant_id, footprint_key, verb, item_key, source_event_type, occurred_at)
			VALUES ($1, $2, $3, $4, $5, 'trail.footprint.v1', $6)`,
			userID, tenantID, fpKey, verb, itemKey, at)
	}

	// Oldest engaged anchor (t0) with a Japanese non-ASCII clean tag ("量子計算").
	// It sits behind >30 newer junk-only anchors seeded below, proving recent_anchors
	// filters out junk-only anchors before applying LIMIT 30.
	seedHomeItem("anchor:quantum", "誤り耐性量子計算の進展", `["量子計算"]`, t0)
	seedFootprint("fp:quantum", "listened", "anchor:quantum", t0)

	// Older engaged anchor (t1) with 3 substantive clean tags.
	seedHomeItem("anchor:deep", "Async I/O with io_uring", `["Rust", "io_uring", "postgresql"]`, t1)
	seedFootprint("fp:deep", "asked", "anchor:deep", t1)

	// More recent engaged anchor (t2) with 1 clean tag ("rust　", padded with
	// ideographic space U+3000) and junk tags including "https\n" with trailing newline.
	seedHomeItem("anchor:recent", "Weekly Tech Digest", `["rust　", "https\n", "ai", "blog", "2025", "[1]", "技術", "a"]`, t2)
	seedFootprint("fp:recent", "read", "anchor:recent", t2)

	// Seed 30 newer engaged footprints whose items carry only junk tags, plus one
	// engaged footprint whose item has a whitespace-only title. Without filtering
	// recent_anchors to clean-tagged, non-blank-titled items before LIMIT 30, these
	// 30 rows would evict anchor:quantum, anchor:deep, and anchor:recent.
	for i := range 30 {
		at := t2.Add(time.Duration(i+1) * time.Second)
		key := fmt.Sprintf("anchor:junk-slot-%02d", i)
		seedHomeItem(key, fmt.Sprintf("Junk Anchor %02d", i), `["https", "https\n", "ai", "blog", "2025", "[1]"]`, at)
		seedFootprint(fmt.Sprintf("fp:junk-slot-%02d", i), "read", key, at)
	}
	seedHomeItem("anchor:whitespace-title", " \t\n\u3000", `["rust", "io_uring", "postgresql", "量子計算"]`, t3)
	seedFootprint("fp:whitespace-title", "read", "anchor:whitespace-title", t3)

	// Most recent footprint (t3), but verb is "dismissed" — must never anchor a candidate.
	seedHomeItem("anchor:dismissed", "Dismissed Kubernetes Notes", `["kubernetes", "ebpf"]`, t3)
	seedFootprint("fp:dismissed", "dismissed", "anchor:dismissed", t3)

	// 1. Target whose only overlap with an anchor is junk tags (including "https\n") — must not be a candidate.
	seedHomeItem("target:junk-only", "Generic AI Blog Post", `["https", "https\n", "AI", "blog", "2025", "[1]", "技術", "a"]`, t3)

	// 2. Target with a whitespace-only title (including ideographic space) and 3 clean shared tags.
	// Must be excluded in SQL so it cannot occupy a LIMIT slot.
	seedHomeItem("target:whitespace-title", " \t\n\u00a0\u3000", `["rust", "io_uring", "postgresql"]`, t3)

	// 3. Target with 3 clean shared tags on anchor:deep and 1 clean shared tag on anchor:recent.
	// Must rank above 1-tag candidates and return anchor:deep (largest clean overlap).
	seedHomeItem("target:more-clean", "Zero-Copy Postgres over io_uring in Rust", `["rust", "io_uring", "postgresql"]`, t3)

	// 4. Target with 1 clean shared tag ("rust") matching "rust　" (U+3000) on anchor:recent,
	// plus "https\n" and other junk tags. Must rank below target:more-clean, treat "https\n"
	// as junk, and tie-break its 1-tag clean overlap in favor of anchor:recent (t2 > t1).
	seedHomeItem("target:fewer-clean", "Rust CLI Notes", `["rust", "https\n", "ai", "blog", "2025"]`, t3)

	// 5. Target whose only shared tag is the non-ASCII Japanese clean tag ("量子計算") on anchor:quantum.
	seedHomeItem("target:quantum", "冷却原子型量子コンピュータのアーキテクチャ", `["量子計算"]`, t3)

	// 6. Target that already has a cluster branch — must be excluded by NOT EXISTS.
	seedHomeItem("target:already-branched", "Already Proposed Rust Storage", `["rust", "io_uring", "postgresql"]`, t3)
	pgExec(ctx, t, db, `INSERT INTO knowledge_trail_branches
		(user_id, tenant_id, branch_key, anchor_item_key, relation_kind, why, confidence, target_item_key, created_at)
		VALUES ($1, $2, $3, 'anchor:deep', 'cluster', 'Because you asked about "Async I/O with io_uring"', 'corroborated', 'target:already-branched', $4)`,
		userID, tenantID, "cluster:"+userID.String()+":target:already-branched", t3)

	// 7. Target overlapping only with the dismissed footprint — must not be a candidate.
	seedHomeItem("target:from-dismissed", "eBPF Networking in Kubernetes", `["kubernetes", "ebpf"]`, t3)

	got, err := NewRepository(db).DeriveTrailClusterCandidates(ctx, userID, 3)
	require.NoError(t, err)

	targetKeys := make([]string, 0, len(got))
	for _, c := range got {
		targetKeys = append(targetKeys, c.TargetItemKey)
	}
	assert.NotContains(t, targetKeys, "target:junk-only",
		"a target whose only overlap with the anchor is junk tags (including https\\n) must not be a candidate")
	assert.NotContains(t, targetKeys, "target:whitespace-title",
		"a target with a whitespace-only title must be excluded before LIMIT")
	assert.NotContains(t, targetKeys, "target:already-branched",
		"a target that already has a cluster branch must be excluded")
	assert.NotContains(t, targetKeys, "target:from-dismissed",
		"footprints with verb dismissed must not create anchors")

	require.Equal(t, []string{"target:more-clean", "target:fewer-clean", "target:quantum"}, targetKeys,
		"candidates must rank by clean tag overlap, then anchor recency, including non-ASCII tags beyond 30 junk anchors")

	assert.Equal(t, "anchor:deep", got[0].AnchorItemKey,
		"the anchor returned must be the one with the largest clean overlap")
	assert.Equal(t, "Async I/O with io_uring", got[0].AnchorTitle)
	assert.Equal(t, "asked", got[0].AnchorVerb)
	assert.ElementsMatch(t, []string{"rust", "io_uring", "postgresql"}, got[0].SharedTags)

	assert.Equal(t, "anchor:recent", got[1].AnchorItemKey,
		"ties in clean overlap must break by the most recent engagement, matching ideographic-space-trimmed rust")
	assert.Equal(t, "Weekly Tech Digest", got[1].AnchorTitle)
	assert.Equal(t, "read", got[1].AnchorVerb)
	assert.Equal(t, []string{"rust"}, got[1].SharedTags)

	assert.Equal(t, "anchor:quantum", got[2].AnchorItemKey,
		"non-ASCII clean tag on an anchor older than 30 junk-only anchors must still match")
	assert.Equal(t, "誤り耐性量子計算の進展", got[2].AnchorTitle)
	assert.Equal(t, "listened", got[2].AnchorVerb)
	assert.Equal(t, []string{"量子計算"}, got[2].SharedTags)
}
