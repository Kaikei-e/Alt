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
