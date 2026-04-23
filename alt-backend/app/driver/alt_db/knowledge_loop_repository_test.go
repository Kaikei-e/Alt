package alt_db

import (
	"alt/domain"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func newTestLoopEntry(seq int64) *domain.KnowledgeLoopEntry {
	lensVer := "lens-v1"
	return &domain.KnowledgeLoopEntry{
		UserID:               uuid.New(),
		TenantID:             uuid.New(),
		LensModeID:           "default",
		EntryKey:             "article:42",
		SourceItemKey:        "article:42",
		ProposedStage:        domain.LoopStageObserve,
		SurfaceBucket:        domain.SurfaceNow,
		ProjectionSeqHiwater: seq,
		SourceEventSeq:       seq,
		FreshnessAt:          time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
		ArtifactVersionRef: domain.ArtifactVersionRef{
			LensVersionID: &lensVer,
		},
		WhyKind:         domain.WhyKindSource,
		WhyText:         "new unread article",
		DismissState:    domain.DismissActive,
		RenderDepthHint: domain.RenderDepthLight,
		LoopPriority:    domain.LoopPriorityCritical,
	}
}

// TestUpsertKnowledgeLoopEntry_FirstInsert exercises the INSERT branch of the UPSERT.
// The first write for a (user, lens, entry_key) triple MUST return Applied=true.
func TestUpsertKnowledgeLoopEntry_FirstInsert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &KnowledgeLoopRepository{pool: mock}
	entry := newTestLoopEntry(100)

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

// TestUpsertKnowledgeLoopEntry_SeqHiwaterGuard_SkipsStaleReplay verifies the out-of-order
// replay guard. An UPSERT whose projection_seq_hiwater is below the existing row's
// must be a no-op (WHERE guard returns zero rows → pgx.ErrNoRows).
// This is the security/correctness-critical path: without this, old events would
// overwrite newer projection state during reproject.
func TestUpsertKnowledgeLoopEntry_SeqHiwaterGuard_SkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &KnowledgeLoopRepository{pool: mock}
	staleEntry := newTestLoopEntry(50) // older event seq than the hypothetical existing row

	anyArgs := make([]interface{}, 27)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), staleEntry)
	require.NoError(t, err, "stale seq must not be an error, it is a design-level skip")
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(0), res.ProjectionRevision)
	require.Equal(t, int64(0), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReserveTransitionIdempotency_FreshClaim exercises a fresh idempotency key.
func TestReserveTransitionIdempotency_FreshClaim(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &KnowledgeLoopRepository{pool: mock}
	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(reserveTransitionIdempotencyQuery)).
		WithArgs(userID, "01JZA0000000000000000000000").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(userID))

	reserved, cached, err := repo.ReserveTransitionIdempotency(context.Background(), userID, "01JZA0000000000000000000000")
	require.NoError(t, err)
	require.True(t, reserved)
	require.Nil(t, cached)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReserveTransitionIdempotency_Duplicate verifies cached replay on duplicate key.
// TTL-window replays return the cached response so the event log is never re-appended.
func TestReserveTransitionIdempotency_Duplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &KnowledgeLoopRepository{pool: mock}
	userID := uuid.New()
	clientTxID := "01JZA0000000000000000000001"
	canonical := "article:42"

	mock.ExpectQuery(regexp.QuoteMeta(reserveTransitionIdempotencyQuery)).
		WithArgs(userID, clientTxID).
		WillReturnError(pgx.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(loadCachedTransitionResponseQuery)).
		WithArgs(userID, clientTxID).
		WillReturnRows(pgxmock.NewRows([]string{"canonical_entry_key", "response_payload", "created_at"}).
			AddRow(&canonical, []byte(`{"accepted":false}`), time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)))

	reserved, cached, err := repo.ReserveTransitionIdempotency(context.Background(), userID, clientTxID)
	require.NoError(t, err)
	require.False(t, reserved)
	require.NotNil(t, cached)
	require.NotNil(t, cached.CanonicalEntryKey)
	require.Equal(t, "article:42", *cached.CanonicalEntryKey)
	require.NoError(t, mock.ExpectationsWereMet())
}
