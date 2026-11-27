//go:build integration

package reading_projector

// Real PostgreSQL rebuild-vs-projector CAS integration tests for
// reading_projector, mirroring knowledge_home_projector/projector_pg_test.go
// (ADR-000965 / ADR-000967 / ADR-001031 Phase 2).

import (
	"context"
	"encoding/json"
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

var pgReadingEventAt = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

type pgReadingRebuildDuringBatch struct {
	Repository
	rebuild func()
	fired   bool
}

func (r *pgReadingRebuildDuringBatch) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	events, err := r.Repository.ListKnowledgeEventsSince(ctx, afterSeq, limit)
	if err != nil || r.fired {
		return events, err
	}
	r.fired = true
	r.rebuild()
	return events, nil
}

type pgReadingRecordedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

type pgReadingRecordingHandler struct {
	records []pgReadingRecordedLog
}

func (h *pgReadingRecordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *pgReadingRecordingHandler) WithAttrs(_ []slog.Attr) slog.Handler     { return h }
func (h *pgReadingRecordingHandler) WithGroup(_ string) slog.Handler          { return h }
func (h *pgReadingRecordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, pgReadingRecordedLog{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *pgReadingRecordingHandler) find(msg string) (pgReadingRecordedLog, bool) {
	for _, r := range h.records {
		if r.Message == msg {
			return r, true
		}
	}
	return pgReadingRecordedLog{}, false
}

func pgSeedReadingQueuedEvent(ctx context.Context, t *testing.T, db *pgxpool.Pool, tenantID, userID uuid.UUID, n int) int64 {
	t.Helper()
	articleID := uuid.New()
	at := pgReadingEventAt.Add(time.Duration(n) * time.Minute)
	payload, err := json.Marshal(map[string]any{
		"article_id":           articleID.String(),
		"queued_at":            at.Format(time.RFC3339),
		"expires_at":           at.Add(48 * time.Hour).Format(time.RFC3339),
		"queue_policy_version": 1,
	})
	require.NoError(t, err)

	var seq int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO knowledge_events
		(occurred_at, tenant_id, user_id, actor_type, actor_id, event_type, aggregate_type, aggregate_id, dedupe_key, payload)
		VALUES ($1, $2, $3, 'user', '', $4, 'article', $5, $6, $7)
		RETURNING event_seq`,
		at, tenantID, userID, eventReadingQueued,
		"article:"+articleID.String(), fmt.Sprintf("reading-queued-%d", n), payload,
	).Scan(&seq))
	return seq
}

func pgCountReadingQueueItems(ctx context.Context, t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM reading_queue_items").Scan(&n))
	return n
}

func pgReadingCheckpointSeq(ctx context.Context, t *testing.T, db *pgxpool.Pool) (int64, bool) {
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

func TestProjector_InFlightBatchMustNotOverwriteAConcurrentRebuildsCheckpointReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	repo := sovereign_db.NewRepository(db)
	tenantID, userID := uuid.New(), uuid.New()

	for i := 1; i <= 3; i++ {
		pgSeedReadingQueuedEvent(ctx, t, db, tenantID, userID, i)
	}
	require.NoError(t, NewProjector(repo, slog.New(&pgReadingRecordingHandler{}), Config{}).RunBatch(ctx))

	settled, exists := pgReadingCheckpointSeq(ctx, t, db)
	require.True(t, exists, "precondition: initial tick must leave a checkpoint row")
	require.NotZero(t, settled, "precondition: racing batch starts from non-zero checkpoint")
	require.EqualValues(t, 3, pgCountReadingQueueItems(ctx, t, db), "precondition: three queue items projected")

	var tip int64
	for i := 4; i <= 5; i++ {
		tip = pgSeedReadingQueuedEvent(ctx, t, db, tenantID, userID, i)
	}

	target, err := sovereign_db.LookupRebuildTarget("reading")
	require.NoError(t, err)

	logs := &pgReadingRecordingHandler{}
	racing := &pgReadingRebuildDuringBatch{Repository: repo, rebuild: func() {
		_, err := repo.RebuildProjection(ctx, target)
		require.NoError(t, err)
	}}
	require.NoError(t, NewProjector(racing, slog.New(logs), Config{}).RunBatch(ctx),
		"losing the checkpoint race is recoverable, not a batch failure")
	require.True(t, racing.fired, "rebuild must have run inside the batch")

	seq, exists := pgReadingCheckpointSeq(ctx, t, db)
	require.True(t, exists, "rebuild leaves a checkpoint row behind")
	assert.Zero(t, seq,
		"rebuild reset checkpoint to 0; stale batch that read %d must not advance it to %d",
		settled, tip)

	rejected, ok := logs.find("reading_projector.checkpoint_advance_rejected")
	require.True(t, ok, "rejected checkpoint advance must be reported, never swallowed")
	assert.Equal(t, slog.LevelError, rejected.Level)
	assert.EqualValues(t, settled, rejected.Attrs["expected_seq"])
	assert.EqualValues(t, tip, rejected.Attrs["attempted_seq"])

	// Next tick re-folds the full log across the initial burned hole in a single tick.
	require.NoError(t, NewProjector(repo, slog.New(&pgReadingRecordingHandler{}), Config{}).RunBatch(ctx))
	assert.EqualValues(t, 5, pgCountReadingQueueItems(ctx, t, db))
	seq, _ = pgReadingCheckpointSeq(ctx, t, db)
	assert.Equal(t, tip, seq)
}
