//go:build integration

package sovereign_db

import (
	"context"
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

	t1 := pgSeedAt
	t2 := pgSeedAt.Add(10 * time.Minute)
	t3 := pgSeedAt.Add(20 * time.Minute)

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

	// Older engaged anchor (t1) with 3 substantive clean tags.
	seedHomeItem("anchor:deep", "Async I/O with io_uring", `["Rust", "io_uring", "postgresql"]`, t1)
	seedFootprint("fp:deep", "asked", "anchor:deep", t1)

	// More recent engaged anchor (t2) with 1 clean tag ("rust", padded with whitespace)
	// and several junk tags (URL debris, generic meta-tags, digits, symbols, short runes).
	seedHomeItem("anchor:recent", "Weekly Tech Digest", `[" rust ", "https", "ai", "blog", "2025", "[1]", "技術", "a"]`, t2)
	seedFootprint("fp:recent", "read", "anchor:recent", t2)

	// Most recent footprint (t3), but verb is "dismissed" — must never anchor a candidate.
	seedHomeItem("anchor:dismissed", "Dismissed Kubernetes Notes", `["kubernetes", "ebpf"]`, t3)
	seedFootprint("fp:dismissed", "dismissed", "anchor:dismissed", t3)

	// 1. Target whose only overlap with an anchor is junk tags — must not be a candidate.
	seedHomeItem("target:junk-only", "Generic AI Blog Post", `["https", "AI", "blog", "2025", "[1]", "技術", "a"]`, t3)

	// 2. Target with 3 clean shared tags on anchor:deep and 1 clean shared tag on anchor:recent.
	// Must rank above target:fewer-clean and return anchor:deep (largest clean overlap).
	seedHomeItem("target:more-clean", "Zero-Copy Postgres over io_uring in Rust", `["rust", "io_uring", "postgresql"]`, t3)

	// 3. Target with 1 clean shared tag ("rust") + 4 junk tags on anchor:recent, and 1 clean
	// shared tag ("rust") on anchor:deep. Must rank below target:more-clean, and tie-break
	// its 1-tag clean overlap in favor of the more recent anchor (anchor:recent).
	seedHomeItem("target:fewer-clean", "Rust CLI Notes", `["Rust", "https", "ai", "blog", "2025"]`, t3)

	// 4. Target that already has a cluster branch — must be excluded by NOT EXISTS.
	seedHomeItem("target:already-branched", "Already Proposed Rust Storage", `["rust", "io_uring", "postgresql"]`, t3)
	pgExec(ctx, t, db, `INSERT INTO knowledge_trail_branches
		(user_id, tenant_id, branch_key, anchor_item_key, relation_kind, why, confidence, target_item_key, created_at)
		VALUES ($1, $2, $3, 'anchor:deep', 'cluster', 'Because you asked about "Async I/O with io_uring"', 'corroborated', 'target:already-branched', $4)`,
		userID, tenantID, "cluster:"+userID.String()+":target:already-branched", t3)

	// 5. Target overlapping only with the dismissed footprint — must not be a candidate.
	seedHomeItem("target:from-dismissed", "eBPF Networking in Kubernetes", `["kubernetes", "ebpf"]`, t3)

	got, err := NewRepository(db).DeriveTrailClusterCandidates(ctx, userID, 10)
	require.NoError(t, err)

	targetKeys := make([]string, 0, len(got))
	for _, c := range got {
		targetKeys = append(targetKeys, c.TargetItemKey)
	}
	assert.NotContains(t, targetKeys, "target:junk-only",
		"a target whose only overlap with the anchor is junk tags must not be a candidate")
	assert.NotContains(t, targetKeys, "target:already-branched",
		"a target that already has a cluster branch must be excluded")
	assert.NotContains(t, targetKeys, "target:from-dismissed",
		"footprints with verb dismissed must not create anchors")

	require.Equal(t, []string{"target:more-clean", "target:fewer-clean"}, targetKeys,
		"target with more clean shared tags must rank above one with fewer")

	assert.Equal(t, "anchor:deep", got[0].AnchorItemKey,
		"the anchor returned must be the one with the largest clean overlap")
	assert.Equal(t, "Async I/O with io_uring", got[0].AnchorTitle)
	assert.Equal(t, "asked", got[0].AnchorVerb)
	assert.ElementsMatch(t, []string{"rust", "io_uring", "postgresql"}, got[0].SharedTags)

	assert.Equal(t, "anchor:recent", got[1].AnchorItemKey,
		"ties in clean overlap must break by the most recent engagement")
	assert.Equal(t, "Weekly Tech Digest", got[1].AnchorTitle)
	assert.Equal(t, "read", got[1].AnchorVerb)
	assert.Equal(t, []string{"rust"}, got[1].SharedTags)
}
