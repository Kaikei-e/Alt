package sovereign_db

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetOpenTrailBranchesForAnchor_FiltersByAnchorAndLimits pins the Wave 10
// (D26) patch-exit read shape: branches narrow to one anchor item and are
// capped server-side, mirroring GetOpenTrailBranches but scoped to the
// article the user just finished reading.
func TestGetOpenTrailBranchesForAnchor_FiltersByAnchorAndLimits(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	_, err := repo.GetOpenTrailBranchesForAnchor(context.Background(), userID, "article:1", 2)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one anchored-branches query")

	call := mock.queryCalls[0]
	assert.Contains(t, call.SQL, "b.anchor_item_key = $2",
		"the query must narrow to the anchor item, not the whole open set")
	assert.Contains(t, call.SQL, "b.state = 'open'",
		"only open branches surface")
	assert.Contains(t, call.SQL, "LIMIT $3",
		"the patch-exit surface is capped server-side (D26 — max 1-2)")
	require.Len(t, call.Args, 3)
	assert.Equal(t, userID, call.Args[0])
	assert.Equal(t, "article:1", call.Args[1])
	assert.Equal(t, 2, call.Args[2])
}

// TestGetItemTitle_QueriesByUserAndItemKey pins the small read the trail
// planner uses to resolve an anchor's display title (D28 — anchored why): a
// single-row lookup scoped to the user and item key.
func TestGetItemTitle_QueriesByUserAndItemKey(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	var gotSQL string
	var gotArgs []interface{}
	mock.queryRowFunc = func(_ context.Context, sql string, args ...interface{}) pgx.Row {
		gotSQL = sql
		gotArgs = args
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*string); ok {
				*p = "Async Rust"
			}
			return nil
		}}
	}

	userID := uuid.New()
	title, ok, err := repo.GetItemTitle(context.Background(), userID, "article:1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Async Rust", title)
	assert.Contains(t, gotSQL, "knowledge_home_items")
	assert.Contains(t, gotSQL, "item_key = $2")
	require.Len(t, gotArgs, 2)
	assert.Equal(t, userID, gotArgs[0])
	assert.Equal(t, "article:1", gotArgs[1])
}

// TestGetItemTitle_NoRowsReturnsNotOK pins the "cannot name it" branch: a
// missing row must report ok=false, not a fabricated title or an error the
// planner would have to distinguish from a real failure.
func TestGetItemTitle_NoRowsReturnsNotOK(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			return pgx.ErrNoRows
		}}
	}
	repo := &Repository{pool: mock}

	title, ok, err := repo.GetItemTitle(context.Background(), uuid.New(), "article:missing")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, title)
}

// TestGetLatestFootprintAnchor_OnlyEngagementVerbsCanAnchor pins core-concept
// §C4 (anchored why): the anchor a branch forks from must be an act the why
// can truthfully name — "you read / listened to / asked about this". A
// `dismissed` footprint is a refusal, so it can never anchor a branch, no
// matter how recent it is. The verb travels with the anchor so the why can be
// phrased from the act that actually happened.
func TestGetLatestFootprintAnchor_OnlyEngagementVerbsCanAnchor(t *testing.T) {
	mock := &mockPgx{}
	var gotSQL string
	var gotArgs []interface{}
	mock.queryRowFunc = func(_ context.Context, sql string, args ...interface{}) pgx.Row {
		gotSQL = sql
		gotArgs = args
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*string); ok {
				*p = "article:1"
			}
			if p, ok := dest[2].(*string); ok {
				*p = "asked"
			}
			return nil
		}}
	}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	anchor, ok, err := repo.GetLatestFootprintAnchor(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, ok)

	assert.Contains(t, gotSQL, "verb = ANY($2::text[])",
		"only verbs that can support an anchored why may anchor a branch")
	require.Len(t, gotArgs, 2)
	assert.Equal(t, userID, gotArgs[0])
	assert.Equal(t, EngagementVerbs, gotArgs[1])
	assert.NotContains(t, EngagementVerbs, "dismissed",
		"a dismissal is a refusal — it can never back a 'because you read this' claim")

	assert.Equal(t, "article:1", anchor.ItemKey)
	assert.Equal(t, "asked", anchor.Verb, "the why must be phrased from the act that happened")
}

func TestScanTrailBranchRow(t *testing.T) {
	tests := []struct {
		name       string
		branchKey  string
		anchorKey  string
		kind       string
		why        string
		refsJSON   []byte
		confidence string
		targetKey  string
		targetTit  string
		scanErr    error
		wantErr    bool
		wantBranch TrailBranch
	}{
		{
			name:       "valid branch with refs",
			branchKey:  "br-101",
			anchorKey:  "art-1",
			kind:       "continuation",
			why:        "Because you read art-1",
			refsJSON:   []byte(`[{"ref_id":"ref-1","label":"Ref Label","kind":"item"}]`),
			confidence: "high",
			targetKey:  "art-2",
			targetTit:  "Article Two",
			wantErr:    false,
			wantBranch: TrailBranch{
				BranchKey:     "br-101",
				AnchorItemKey: "art-1",
				RelationKind:  "continuation",
				Why:           "Because you read art-1",
				EvidenceRefs: []TrailEvidenceRef{
					{RefID: "ref-1", Label: "Ref Label", Kind: "item"},
				},
				Confidence:    "high",
				TargetItemKey: "art-2",
				TargetTitle:   "Article Two",
			},
		},
		{
			name:    "scan error",
			scanErr: errors.New("db scan error"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRow{
				scanFunc: func(dest ...interface{}) error {
					if tc.scanErr != nil {
						return tc.scanErr
					}
					*dest[0].(*string) = tc.branchKey
					*dest[1].(*string) = tc.anchorKey
					*dest[2].(*string) = tc.kind
					*dest[3].(*string) = tc.why
					*dest[4].(*[]byte) = tc.refsJSON
					*dest[5].(*string) = tc.confidence
					*dest[6].(*string) = tc.targetKey
					*dest[7].(*string) = tc.targetTit
					return nil
				},
			}

			b, err := scanTrailBranchRow(mock)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantBranch, b)
		})
	}
}
