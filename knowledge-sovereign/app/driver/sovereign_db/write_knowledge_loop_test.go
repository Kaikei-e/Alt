package sovereign_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func newLoopEntryProto(seq int64) *sovereignv1.KnowledgeLoopEntry {
	lensVer := "lens-v1"
	return &sovereignv1.KnowledgeLoopEntry{
		UserId:               uuid.New().String(),
		TenantId:             uuid.New().String(),
		LensModeId:           "default",
		EntryKey:             "article:42",
		SourceItemKey:        "article:42",
		ProposedStage:        sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
		SurfaceBucket:        sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		ProjectionSeqHiwater: seq,
		SourceEventSeq:       seq,
		FreshnessAt:          timestamppb.New(time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)),
		ArtifactVersionRef: &sovereignv1.KnowledgeLoopArtifactVersionRef{
			LensVersionId: &lensVer,
		},
		WhyPrimary: &sovereignv1.KnowledgeLoopWhyPayload{
			Kind: sovereignv1.WhyKind_WHY_KIND_SOURCE,
			Text: "new unread article",
		},
		DismissState:    sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
		RenderDepthHint: 2,
		LoopPriority:    sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL,
	}
}

// TestUpsertKnowledgeLoopEntry_Insert exercises the INSERT branch.
func TestUpsertKnowledgeLoopEntry_Insert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	entry := newLoopEntryProto(100)

	anyArgs := make([]interface{}, 27)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(1), int64(100)))

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), entry)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(1), res.ProjectionRevision)
	require.Equal(t, int64(100), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpsertKnowledgeLoopEntry_SeqHiwaterGuardSkipsStaleReplay verifies the seq-hiwater
// guard: an event with an older seq than the existing row's returns SkippedBySeqHiwater=true.
// This is the reproject-safety invariant — replaying historical events is a no-op when the
// projection has already advanced past them.
func TestUpsertKnowledgeLoopEntry_SeqHiwaterGuardSkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	stale := newLoopEntryProto(50)

	anyArgs := make([]interface{}, 27)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), stale)
	require.NoError(t, err, "stale seq must surface as SkippedBySeqHiwater, not as an error")
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReserveKnowledgeLoopTransition_Fresh exercises a first-time idempotency claim.
func TestReserveKnowledgeLoopTransition_Fresh(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(reserveKnowledgeLoopTransitionQuery)).
		WithArgs(userID, "01JZA0000000000000000000000").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(userID))

	res, err := repo.ReserveKnowledgeLoopTransition(context.Background(), userID, "01JZA0000000000000000000000")
	require.NoError(t, err)
	require.True(t, res.Reserved)
	require.Nil(t, res.CanonicalEntryKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReserveKnowledgeLoopTransition_Duplicate verifies that a duplicate key returns the cached
// response so the caller can replay it without re-appending to knowledge_events.
func TestReserveKnowledgeLoopTransition_Duplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	clientTxID := "01JZA0000000000000000000001"
	canonical := "article:42"
	created := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(reserveKnowledgeLoopTransitionQuery)).
		WithArgs(userID, clientTxID).
		WillReturnError(pgx.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(loadCachedKnowledgeLoopTransitionQuery)).
		WithArgs(userID, clientTxID).
		WillReturnRows(pgxmock.NewRows([]string{"canonical_entry_key", "response_payload", "created_at"}).
			AddRow(&canonical, []byte(`{"accepted":false}`), &created))

	res, err := repo.ReserveKnowledgeLoopTransition(context.Background(), userID, clientTxID)
	require.NoError(t, err)
	require.False(t, res.Reserved)
	require.NotNil(t, res.CanonicalEntryKey)
	require.Equal(t, "article:42", *res.CanonicalEntryKey)
	require.NoError(t, mock.ExpectationsWereMet())
}
