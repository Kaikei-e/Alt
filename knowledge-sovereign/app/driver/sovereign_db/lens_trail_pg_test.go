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

func TestAreArticlesVisibleInLens_FiltersAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	otherTenantID := uuid.New()

	now := time.Now().UTC().Truncate(time.Second)
	recent := now.Add(-2 * 24 * time.Hour)
	old := now.Add(-45 * 24 * time.Hour)

	artRust := uuid.New()
	artGo := uuid.New()
	artDismissed := uuid.New()
	artOld := uuid.New()
	artOtherTenant := uuid.New()
	artMissing := uuid.New()

	seedArticle := func(tID uuid.UUID, refID uuid.UUID, title, excerpt, tagsJSON string, publishedAt time.Time, dismissedAt *time.Time) {
		pgExec(ctx, t, db, `INSERT INTO knowledge_home_items
			(user_id, tenant_id, item_key, item_type, primary_ref_id, title, summary_excerpt, tags_json, published_at, generated_at, updated_at, dismissed_at, projection_version)
			VALUES ($1, $2, $3, 'article', $4, $5, $6, $7::jsonb, $8, $8, $8, $9, 1)`,
			userID, tID, "article:"+refID.String(), refID, title, excerpt, tagsJSON, publishedAt, dismissedAt)
	}

	seedArticle(tenantID, artRust, "Async Rust Runtime", "Tokio scheduler internals", `["rust", "tokio"]`, recent, nil)
	seedArticle(tenantID, artGo, "Go Memory Model", "Happens-before guarantees in channels", `["golang", "concurrency"]`, recent, nil)
	seedArticle(tenantID, artDismissed, "Dismissed Rust Guide", "Tokio basics", `["rust", "tokio"]`, recent, &now)
	seedArticle(tenantID, artOld, "Historical Systems Paper", "Early microkernel notes", `["rust", "systems"]`, old, nil)
	seedArticle(otherTenantID, artOtherTenant, "Other Tenant Rust Article", "Tokio notes", `["rust", "tokio"]`, recent, nil)

	allIDs := []uuid.UUID{artRust, artGo, artDismissed, artOld, artOtherTenant, artMissing}

	// 1. Unfiltered (nil filter): returns non-dismissed articles for (tenantID, userID) only.
	visible, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, allIDs, nil)
	require.NoError(t, err)
	assert.True(t, visible[artRust], "active Rust article must be visible")
	assert.True(t, visible[artGo], "active Go article must be visible")
	assert.True(t, visible[artOld], "older active article must be visible when no time window is set")
	assert.False(t, visible[artDismissed], "dismissed article must be excluded")
	assert.False(t, visible[artOtherTenant], "article from another tenant must be excluded")
	assert.False(t, visible[artMissing], "unknown article ID must be absent (fail-closed)")

	// 2. Tag filter + SourceIDs populated: matches only non-dismissed articles carrying the tag.
	visible, err = repo.AreArticlesVisibleInLens(ctx, tenantID, userID, allIDs, &LensFilter{
		TagNames:  []string{"rust"},
		SourceIDs: []string{"source-alpha"},
	})
	require.NoError(t, err)
	assert.True(t, visible[artRust])
	assert.True(t, visible[artOld])
	assert.False(t, visible[artGo], "article without matching tag must be excluded")
	assert.False(t, visible[artDismissed], "dismissed article with matching tag must still be excluded")

	// 3. QueryText ILIKE across title, summary_excerpt, and tags_json.
	visibleByExcerpt, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, allIDs, &LensFilter{
		QueryText: "HAPPENS-BEFORE",
	})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]bool{artGo: true}, visibleByExcerpt,
		"case-insensitive ILIKE on summary_excerpt must match only artGo")

	visibleByTagText, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, allIDs, &LensFilter{
		QueryText: "TOKIO",
	})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]bool{artRust: true}, visibleByTagText,
		"case-insensitive ILIKE on tags_json must match artRust while excluding dismissed artDismissed")

	// 4. Combined TagNames + TimeWindow ("7d") excludes old articles.
	visibleRecentRust, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, allIDs, &LensFilter{
		TagNames:   []string{"rust"},
		TimeWindow: "7d",
	})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]bool{artRust: true}, visibleRecentRust,
		"7d time window must exclude 45-day-old artOld")
}

func TestGetTrailFootprints_EngagementVerbsAndWearAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	baseAt := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)

	seedHomeItem := func(itemKey, title string) {
		pgExec(ctx, t, db, `INSERT INTO knowledge_home_items
			(user_id, tenant_id, item_key, item_type, title, summary_excerpt, tags_json, generated_at, updated_at, projection_version)
			VALUES ($1, $2, $3, 'article', $4, 'excerpt', '["rust"]'::jsonb, $5, $5, 1)`,
			userID, tenantID, itemKey, title, baseAt)
	}

	// 1. "item:thin" — single "read" footprint, dwell below engaged threshold (10s < 30s) -> wear "thin".
	seedHomeItem("item:thin", "Thin Read Article")
	require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
		UserID:          userID,
		TenantID:        tenantID,
		FootprintKey:    "fp:thin:1",
		Verb:            "read",
		ItemKey:         "item:thin",
		SourceEventType: "HomeItemOpened",
		OccurredAt:      baseAt.Add(1 * time.Minute),
	}, 2))
	shortDwell := int64(10_000)
	require.NoError(t, repo.InsertTrailActOutcome(ctx, TrailActOutcome{
		UserID:          userID,
		TenantID:        tenantID,
		OutcomeKey:      "out:thin:1",
		BranchKey:       "branch:thin",
		ItemKey:         "item:thin",
		DwellMs:         &shortDwell,
		SourceEventType: "trail.act_outcome.v1",
		OccurredAt:      baseAt.Add(2 * time.Minute),
	}, 2))

	// 2. "item:worn-dwell" — single "listened" footprint, dwell >= 30000ms -> wear "worn".
	seedHomeItem("item:worn-dwell", "Worn By Dwell Article")
	require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
		UserID:          userID,
		TenantID:        tenantID,
		FootprintKey:    "fp:worn-dwell:1",
		Verb:            "listened",
		ItemKey:         "item:worn-dwell",
		SourceEventType: "HomeItemListened",
		OccurredAt:      baseAt.Add(3 * time.Minute),
	}, 2))
	engagedDwell := int64(35_000)
	require.NoError(t, repo.InsertTrailActOutcome(ctx, TrailActOutcome{
		UserID:          userID,
		TenantID:        tenantID,
		OutcomeKey:      "out:worn-dwell:1",
		BranchKey:       "branch:worn-dwell",
		ItemKey:         "item:worn-dwell",
		DwellMs:         &engagedDwell,
		SourceEventType: "trail.act_outcome.v1",
		OccurredAt:      baseAt.Add(4 * time.Minute),
	}, 2))

	// 3. "item:worn-contacts" — 2 "read" footprints, no engaged outcome -> collapsed contact_count=2, wear "worn".
	seedHomeItem("item:worn-contacts", "Worn By Revisit Article")
	for i := range 2 {
		require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
			UserID:          userID,
			TenantID:        tenantID,
			FootprintKey:    fmt.Sprintf("fp:worn-contacts:%d", i),
			Verb:            "read",
			ItemKey:         "item:worn-contacts",
			SourceEventType: "HomeItemOpened",
			OccurredAt:      baseAt.Add(time.Duration(5+i) * time.Minute),
		}, 2))
	}

	// 4. "item:deep-ask" — single "asked" footprint -> wear "deep".
	seedHomeItem("item:deep-ask", "Deep By Ask Article")
	require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
		UserID:          userID,
		TenantID:        tenantID,
		FootprintKey:    "fp:deep-ask:1",
		Verb:            "asked",
		ItemKey:         "item:deep-ask",
		SourceEventType: "HomeItemAsked",
		OccurredAt:      baseAt.Add(10 * time.Minute),
	}, 2))

	// 5. "item:deep-contacts" — 4 "read" footprints -> collapsed contact_count=4, wear "deep".
	seedHomeItem("item:deep-contacts", "Deep By Four Contacts Article")
	for i := range 4 {
		require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
			UserID:          userID,
			TenantID:        tenantID,
			FootprintKey:    fmt.Sprintf("fp:deep-contacts:%d", i),
			Verb:            "read",
			ItemKey:         "item:deep-contacts",
			SourceEventType: "HomeItemOpened",
			OccurredAt:      baseAt.Add(time.Duration(15+i) * time.Minute),
		}, 2))
	}

	// 6. "item:dismissed" — "dismissed" footprint must be excluded from the spine.
	seedHomeItem("item:dismissed", "Dismissed Article")
	require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
		UserID:          userID,
		TenantID:        tenantID,
		FootprintKey:    "fp:dismissed:1",
		Verb:            "dismissed",
		ItemKey:         "item:dismissed",
		SourceEventType: "HomeItemDismissed",
		OccurredAt:      baseAt.Add(30 * time.Minute),
	}, 2))

	fps, _, hasMore, err := repo.GetTrailFootprints(ctx, userID, "", 20, nil)
	require.NoError(t, err)
	assert.False(t, hasMore)

	byItem := make(map[string]TrailFootprint, len(fps))
	for _, fp := range fps {
		byItem[fp.ItemKey] = fp
	}

	assert.NotContains(t, byItem, "item:dismissed", "dismissed footprints must not appear on the trail spine")
	require.Len(t, fps, 5, "all three engagement verbs (read, asked, listened) must appear, with repeated reads collapsed")

	assert.Equal(t, "thin", byItem["item:thin"].Wear)
	assert.Equal(t, 1, byItem["item:thin"].ContactCount)
	assert.Equal(t, "read", byItem["item:thin"].Verb)

	assert.Equal(t, "worn", byItem["item:worn-dwell"].Wear, "dwell_ms >= 30000 must upgrade wear to worn")
	assert.Equal(t, "listened", byItem["item:worn-dwell"].Verb)

	assert.Equal(t, "worn", byItem["item:worn-contacts"].Wear, "contact_count >= 2 must upgrade wear to worn")
	assert.Equal(t, 2, byItem["item:worn-contacts"].ContactCount)

	assert.Equal(t, "deep", byItem["item:deep-ask"].Wear, "asked verb must upgrade wear to deep")
	assert.Equal(t, "asked", byItem["item:deep-ask"].Verb)

	assert.Equal(t, "deep", byItem["item:deep-contacts"].Wear, "contact_count >= 4 must upgrade wear to deep")
	assert.Equal(t, 4, byItem["item:deep-contacts"].ContactCount)
}

func TestDeriveTrailContinuationCandidates_WindowAndExclusionsAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	fiveDaysAgo := now.Add(-5 * 24 * time.Hour)
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	oneDayAgo := now.Add(-1 * 24 * time.Hour)
	twentyFiveDaysAgo := now.Add(-25 * 24 * time.Hour)

	seedHomeItem := func(itemKey, title string, dismissedAt *time.Time) {
		pgExec(ctx, t, db, `INSERT INTO knowledge_home_items
			(user_id, tenant_id, item_key, item_type, title, generated_at, updated_at, dismissed_at, projection_version)
			VALUES ($1, $2, $3, 'article', $4, $5, $5, $6, 1)`,
			userID, tenantID, itemKey, title, twentyFiveDaysAgo, dismissedAt)
	}
	seedFootprint := func(fpKey, verb, itemKey string, at time.Time) {
		require.NoError(t, repo.UpsertTrailFootprint(ctx, TrailFootprint{
			UserID:          userID,
			TenantID:        tenantID,
			FootprintKey:    fpKey,
			Verb:            verb,
			ItemKey:         itemKey,
			SourceEventType: "HomeItemOpened",
			OccurredAt:      at,
		}, 2))
	}

	// Qualifying 1: 2 read contacts, last contact 5 days ago (within 3..21d), non-engaged outcome (12s < 30s).
	seedHomeItem("cand:qualifying-read", "Quiet Distributed Transactions Thread", nil)
	seedFootprint("fp:qual-read:1", "read", "cand:qualifying-read", fiveDaysAgo.Add(-1*time.Hour))
	seedFootprint("fp:qual-read:2", "read", "cand:qualifying-read", fiveDaysAgo)
	shortDwell := int64(12_000)
	require.NoError(t, repo.InsertTrailActOutcome(ctx, TrailActOutcome{
		UserID:          userID,
		TenantID:        tenantID,
		OutcomeKey:      "out:qual-read",
		BranchKey:       "branch:qual-read",
		ItemKey:         "cand:qualifying-read",
		DwellMs:         &shortDwell,
		SourceEventType: "trail.act_outcome.v1",
		OccurredAt:      fiveDaysAgo,
	}, 2))

	// Qualifying 2: 1 listened contact, last contact 10 days ago (within 3..21d).
	seedHomeItem("cand:qualifying-listened", "Quiet Audio Episode Thread", nil)
	seedFootprint("fp:qual-listen:1", "listened", "cand:qualifying-listened", tenDaysAgo)

	// Excluded 1: too recent (1 day ago < 3 days).
	seedHomeItem("cand:too-recent", "Active Thread Read Yesterday", nil)
	seedFootprint("fp:recent:1", "read", "cand:too-recent", oneDayAgo)

	// Excluded 2: too old (25 days ago > 21 days).
	seedHomeItem("cand:too-old", "Cold Thread From Last Month", nil)
	seedFootprint("fp:old:1", "read", "cand:too-old", twentyFiveDaysAgo)

	// Excluded 3: has "asked" verb (already deep).
	seedHomeItem("cand:has-ask", "Already Asked Thread", nil)
	seedFootprint("fp:ask:1", "read", "cand:has-ask", fiveDaysAgo.Add(-10*time.Minute))
	seedFootprint("fp:ask:2", "asked", "cand:has-ask", fiveDaysAgo)

	// Excluded 4: 4 contacts (already deep, > 3 contacts).
	seedHomeItem("cand:four-contacts", "Four Contacts Thread", nil)
	for i := range 4 {
		seedFootprint(fmt.Sprintf("fp:four:%d", i), "read", "cand:four-contacts", fiveDaysAgo.Add(-time.Duration(i)*time.Minute))
	}

	// Excluded 5: engaged outcome (dwell_ms >= 30000).
	seedHomeItem("cand:engaged-dwell", "Already Substantively Walked Thread", nil)
	seedFootprint("fp:engaged:1", "read", "cand:engaged-dwell", fiveDaysAgo)
	engagedDwell := int64(30_000)
	require.NoError(t, repo.InsertTrailActOutcome(ctx, TrailActOutcome{
		UserID:          userID,
		TenantID:        tenantID,
		OutcomeKey:      "out:engaged",
		BranchKey:       "branch:engaged",
		ItemKey:         "cand:engaged-dwell",
		DwellMs:         &engagedDwell,
		SourceEventType: "trail.act_outcome.v1",
		OccurredAt:      fiveDaysAgo,
	}, 2))

	// Excluded 6: dismissed home item.
	seedHomeItem("cand:dismissed-home", "Dismissed Home Thread", &fiveDaysAgo)
	seedFootprint("fp:dismissed-home:1", "read", "cand:dismissed-home", fiveDaysAgo)

	// Excluded 7: already has a continuation branch.
	seedHomeItem("cand:already-branched", "Already Proposed Continuation Thread", nil)
	seedFootprint("fp:branched:1", "read", "cand:already-branched", fiveDaysAgo)
	require.NoError(t, repo.UpsertTrailBranch(ctx, userID, tenantID, TrailBranch{
		BranchKey:     "continuation:" + userID.String() + ":cand:already-branched",
		AnchorItemKey: "cand:already-branched",
		RelationKind:  "continuation",
		Why:           "Because you read this",
		EvidenceRefs:  []TrailEvidenceRef{{RefID: "cand:already-branched", Label: "Already Proposed Continuation Thread", Kind: "article"}},
		Confidence:    "plausible",
		TargetItemKey: "cand:already-branched",
		TargetTitle:   "Already Proposed Continuation Thread",
	}, fiveDaysAgo, 2))

	got, err := repo.DeriveTrailContinuationCandidates(ctx, userID, 10)
	require.NoError(t, err)

	keys := make([]string, 0, len(got))
	for _, c := range got {
		keys = append(keys, c.TargetItemKey)
	}
	require.Equal(t, []string{"cand:qualifying-read", "cand:qualifying-listened"}, keys,
		"only threads with 1..3 contacts, no ask, no engaged outcome, and last_contact_at between 3 and 21 days ago may qualify, ordered by last_contact_at DESC")

	assert.Equal(t, "Quiet Distributed Transactions Thread", got[0].TargetTitle)
	assert.Equal(t, "read", got[0].Verb)
	assert.True(t, fiveDaysAgo.Equal(got[0].LastContactAt.UTC()))

	assert.Equal(t, "Quiet Audio Episode Thread", got[1].TargetTitle)
	assert.Equal(t, "listened", got[1].Verb)
	assert.True(t, tenDaysAgo.Equal(got[1].LastContactAt.UTC()))
}
