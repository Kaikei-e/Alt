//go:build integration

package sovereign_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

type storedTrailBranchRow struct {
	TenantID          uuid.UUID
	AnchorItemKey     string
	RelationKind      string
	Why               string
	EvidenceRefsJSON  string
	Confidence        string
	TargetItemKey     string
	TargetTitle       string
	State             string
	CreatedAt         time.Time
	ProjectionVersion int
}

func readStoredTrailBranchRow(ctx context.Context, t *testing.T, db *pgxpool.Pool, userID uuid.UUID, branchKey string) storedTrailBranchRow {
	t.Helper()
	var row storedTrailBranchRow
	err := db.QueryRow(ctx, `
		SELECT tenant_id, anchor_item_key, relation_kind, why,
		       evidence_refs_json::text, confidence, target_item_key,
		       target_title, state, created_at, projection_version
		FROM knowledge_trail_branches
		WHERE user_id = $1 AND branch_key = $2`,
		userID, branchKey,
	).Scan(
		&row.TenantID, &row.AnchorItemKey, &row.RelationKind, &row.Why,
		&row.EvidenceRefsJSON, &row.Confidence, &row.TargetItemKey,
		&row.TargetTitle, &row.State, &row.CreatedAt, &row.ProjectionVersion,
	)
	require.NoError(t, err)
	row.CreatedAt = row.CreatedAt.UTC()
	return row
}

func TestUpsertTrailBranch_FirstWriteWinsAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	branchKey := "cluster:" + userID.String() + ":article:target-1"

	firstAt := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	firstProposal := TrailBranch{
		BranchKey:     branchKey,
		AnchorItemKey: "article:anchor-first",
		RelationKind:  "cluster",
		Why:           "Because you read First Anchor — joins rust",
		EvidenceRefs:  []TrailEvidenceRef{{RefID: "rust", Label: "rust", Kind: "tag"}},
		Confidence:    "plausible",
		TargetItemKey: "article:target-1",
		TargetTitle:   "First Target Title",
	}

	secondAt := firstAt.Add(30 * time.Minute)
	secondProposal := TrailBranch{
		BranchKey:     branchKey,
		AnchorItemKey: "article:anchor-second",
		RelationKind:  "cluster",
		Why:           "Because you read Second Anchor — joins distributed-systems",
		EvidenceRefs:  []TrailEvidenceRef{{RefID: "distributed-systems", Label: "distributed-systems", Kind: "tag"}},
		Confidence:    "corroborated",
		TargetItemKey: "article:target-1",
		TargetTitle:   "Second Target Title",
	}

	foldSequence := func() {
		require.NoError(t, repo.UpsertTrailBranch(ctx, userID, tenantID, firstProposal, firstAt, 2))
		require.NoError(t, repo.UpsertTrailBranch(ctx, userID, tenantID, secondProposal, secondAt, 2))
		require.NoError(t, repo.SetTrailBranchState(ctx, userID, branchKey, "dismissed"))
	}

	foldSequence()
	firstPass := readStoredTrailBranchRow(ctx, t, db, userID, branchKey)

	assert.Equal(t, firstProposal.AnchorItemKey, firstPass.AnchorItemKey,
		"duplicate proposal must not overwrite the first proposal's anchor_item_key")
	assert.Equal(t, firstProposal.Why, firstPass.Why,
		"duplicate proposal must not overwrite the first proposal's why")
	assert.True(t, firstAt.Equal(firstPass.CreatedAt),
		"duplicate proposal must not overwrite the first proposal's created_at (got %s, want %s)", firstPass.CreatedAt, firstAt)
	assert.Equal(t, firstProposal.Confidence, firstPass.Confidence,
		"duplicate proposal must not overwrite the first proposal's confidence")
	assert.Equal(t, firstProposal.TargetTitle, firstPass.TargetTitle,
		"duplicate proposal must not overwrite the first proposal's target_title")
	assert.Equal(t, "dismissed", firstPass.State,
		"branch_resolved dismissal must transition state to dismissed")

	// Reproject equivalence: emptying the table and replaying the exact same
	// three events must yield the identical row.
	pgExec(ctx, t, db, `TRUNCATE TABLE knowledge_trail_branches`)
	require.Zero(t, pgCountRows(ctx, t, db, "knowledge_trail_branches"))

	foldSequence()
	reprojected := readStoredTrailBranchRow(ctx, t, db, userID, branchKey)
	assert.Equal(t, firstPass, reprojected,
		"replaying the same proposal + duplicate proposal + resolution into an emptied table must produce the identical row")
}

func TestGetLatestOpenTrailBranchesForAnchors_ScopesAndPicksNewestOpenPerAnchorAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userU := uuid.New()
	tenantU := uuid.New()
	userV := uuid.New()
	tenantV := uuid.New()

	const (
		anchorX = "article:anchor-x"
		anchorY = "article:anchor-y"
		anchorZ = "article:anchor-z"
	)

	baseAt := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

	seedBranch := func(userID, tenantID uuid.UUID, key, anchor, target, title string, createdAt time.Time) {
		require.NoError(t, repo.UpsertTrailBranch(ctx, userID, tenantID, TrailBranch{
			BranchKey:     key,
			AnchorItemKey: anchor,
			RelationKind:  "cluster",
			Why:           "Because you read " + anchor,
			EvidenceRefs:  []TrailEvidenceRef{{RefID: "rust", Label: "rust", Kind: "tag"}},
			Confidence:    "plausible",
			TargetItemKey: target,
			TargetTitle:   title,
		}, createdAt, 1))
	}

	// Anchor X for user U: three open branches at increasing created_at plus a newer dismissed branch.
	seedBranch(userU, tenantU, "br:x-open-1", anchorX, "article:tx-1", "Target X1", baseAt)
	seedBranch(userU, tenantU, "br:x-open-2", anchorX, "article:tx-2", "Target X2", baseAt.Add(10*time.Minute))
	seedBranch(userU, tenantU, "br:x-open-3", anchorX, "article:tx-3", "Target X3", baseAt.Add(30*time.Minute))
	seedBranch(userU, tenantU, "br:x-dismissed-newer", anchorX, "article:tx-4", "Target X4", baseAt.Add(40*time.Minute))
	require.NoError(t, repo.SetTrailBranchState(ctx, userU, "br:x-dismissed-newer", "dismissed"))

	// Anchor Y for user U: one open branch (older than br:x-open-3 so X's newest sorts first).
	seedBranch(userU, tenantU, "br:y-open-1", anchorY, "article:ty-1", "Target Y1", baseAt.Add(20*time.Minute))

	// Anchor Z for user U (not requested): one open branch.
	seedBranch(userU, tenantU, "br:z-open-1", anchorZ, "article:tz-1", "Target Z1", baseAt.Add(50*time.Minute))

	// Another user V with an open branch on anchor X.
	seedBranch(userV, tenantV, "br:v-x-open", anchorX, "article:tv-1", "Target V1", baseAt.Add(60*time.Minute))

	got, err := repo.GetLatestOpenTrailBranchesForAnchors(ctx, userU, []string{anchorX, anchorY})
	require.NoError(t, err)
	require.Len(t, got, 2, "expected exactly the newest open branch for X and the open branch for Y")

	assert.Equal(t, "br:x-open-3", got[0].BranchKey, "newest open branch of anchor X must sort first")
	assert.Equal(t, anchorX, got[0].AnchorItemKey)
	assert.Equal(t, "Target X3", got[0].TargetTitle)

	assert.Equal(t, "br:y-open-1", got[1].BranchKey, "anchor Y's open branch must sort second")
	assert.Equal(t, anchorY, got[1].AnchorItemKey)
	assert.Equal(t, "Target Y1", got[1].TargetTitle)
}
