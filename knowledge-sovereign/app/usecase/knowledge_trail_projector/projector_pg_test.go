//go:build integration

package knowledge_trail_projector

// Real PostgreSQL rebuild-vs-projector CAS integration tests for
// knowledge_trail_projector, mirroring knowledge_home_projector/projector_pg_test.go
// (ADR-000965 / ADR-000967 / ADR-001031 Phase 2).

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/test_utils/pgtest"
)

var pgTrailEventAt = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

type pgTrailRebuildDuringBatch struct {
	Repository
	rebuild func()
	fired   bool
}

func (r *pgTrailRebuildDuringBatch) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	events, err := r.Repository.ListKnowledgeEventsSince(ctx, afterSeq, limit)
	if err != nil || r.fired {
		return events, err
	}
	r.fired = true
	r.rebuild()
	return events, nil
}

func pgSeedTrailOpenEvent(ctx context.Context, t *testing.T, db *pgxpool.Pool, tenantID, userID uuid.UUID, n int) int64 {
	t.Helper()
	itemKey := fmt.Sprintf("article:%s", uuid.NewString())
	var seq int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO knowledge_events
		(occurred_at, tenant_id, user_id, actor_type, actor_id, event_type, aggregate_type, aggregate_id, dedupe_key, payload)
		VALUES ($1, $2, $3, 'user', '', 'HomeItemOpened', 'article', $4, $5, '{}'::jsonb)
		RETURNING event_seq`,
		pgTrailEventAt.Add(time.Duration(n)*time.Minute), tenantID, userID,
		itemKey, fmt.Sprintf("trail-open-%d", n),
	).Scan(&seq))
	return seq
}

func pgCountTrailFootprints(ctx context.Context, t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM knowledge_trail_footprints").Scan(&n))
	return n
}

func pgTrailCheckpointSeq(ctx context.Context, t *testing.T, db *pgxpool.Pool) (int64, bool) {
	t.Helper()
	var seq int64
	err := db.QueryRow(ctx,
		`SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`,
		projectorName).Scan(&seq)
	if err != nil {
		require.ErrorContains(t, err, "no rows")
		return 0, false
	}
	return seq, true
}

// pgTrailDrainFromCheckpointZero folds the full log from checkpoint 0 on one
// projector instance, calling RunBatch twice so parallel cluster XID activity
// from sibling pgtest databases cannot make the initial burned-hole catchup flaky.
func pgTrailDrainFromCheckpointZero(ctx context.Context, t *testing.T, p *Projector) {
	t.Helper()
	require.NoError(t, p.RunBatch(ctx))
	require.NoError(t, p.RunBatch(ctx))
}

func TestProjector_InFlightBatchMustNotOverwriteAConcurrentRebuildsCheckpointReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	repo := sovereign_db.NewRepository(db)
	tenantID, userID := uuid.New(), uuid.New()

	for i := 1; i <= 3; i++ {
		pgSeedTrailOpenEvent(ctx, t, db, tenantID, userID, i)
	}
	pgTrailDrainFromCheckpointZero(ctx, t, NewProjector(repo, slog.New(&recordingHandler{}), Config{}))

	settled, exists := pgTrailCheckpointSeq(ctx, t, db)
	require.True(t, exists, "precondition: initial drain must leave a checkpoint row")
	require.NotZero(t, settled, "precondition: racing batch starts from non-zero checkpoint")
	require.EqualValues(t, 3, pgCountTrailFootprints(ctx, t, db), "precondition: three footprints projected")

	var tip int64
	for i := 4; i <= 5; i++ {
		tip = pgSeedTrailOpenEvent(ctx, t, db, tenantID, userID, i)
	}

	target, err := sovereign_db.LookupRebuildTarget("knowledge-trail")
	require.NoError(t, err)

	logs := &recordingHandler{}
	racing := &pgTrailRebuildDuringBatch{Repository: repo, rebuild: func() {
		_, err := repo.RebuildProjection(ctx, target)
		require.NoError(t, err)
	}}
	require.NoError(t, NewProjector(racing, slog.New(logs), Config{}).RunBatch(ctx),
		"losing the checkpoint race is recoverable, not a batch failure")
	require.True(t, racing.fired, "rebuild must have run inside the batch")

	seq, exists := pgTrailCheckpointSeq(ctx, t, db)
	require.True(t, exists, "rebuild leaves a checkpoint row behind")
	assert.Zero(t, seq,
		"rebuild reset checkpoint to 0; stale batch that read %d must not advance it to %d",
		settled, tip)

	rejected, ok := logs.find("trail.checkpoint_advance_rejected")
	require.True(t, ok, "rejected checkpoint advance must be reported, never swallowed")
	assert.Equal(t, slog.LevelError, rejected.Level)
	assert.EqualValues(t, settled, rejected.Attrs["expected_seq"])
	assert.EqualValues(t, tip, rejected.Attrs["attempted_seq"])

	pgTrailDrainFromCheckpointZero(ctx, t, NewProjector(repo, slog.New(&recordingHandler{}), Config{}))
	assert.EqualValues(t, 5, pgCountTrailFootprints(ctx, t, db))
	seq, _ = pgTrailCheckpointSeq(ctx, t, db)
	assert.Equal(t, tip, seq)
}
