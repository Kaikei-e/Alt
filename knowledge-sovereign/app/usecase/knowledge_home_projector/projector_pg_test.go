//go:build integration

package knowledge_home_projector

// The PM-2026-010 checkpoint race, against a real PostgreSQL carrying the real
// Atlas migration history.
//
// projector_test.go covers every fold against an in-memory fakeRepo whose
// checkpoint is a plain int64 field. That fake cannot express the only thing
// that matters here — another writer moving the checkpoint while this batch is
// folding — so the race is invisible to it by construction. Pinning it needs a
// real server, the real driver/sovereign_db.Repository, and a real
// RebuildProjection running against the same rows.

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

// pgEventAt is a fixed instant, never wall clock: every fold derives its
// business facts from the event's own occurred_at.
var pgEventAt = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

// pgRebuildDuringBatch opens the race window deterministically.
//
// The real window is wall-clock: the projector reads its checkpoint, spends a
// while folding a batch, and only then writes the checkpoint back — and an
// operator's rebuild lands in between. Reproducing that with goroutines and
// sleeps would be flaky and would prove nothing on a fast machine, so the
// rebuild is run exactly once, synchronously, at the point where the projector
// has already read its checkpoint and its batch of events but has not yet
// written the checkpoint back. That is t1..t3 of the PM-2026-010 table, with
// the timing taken out.
//
// The seam is ListKnowledgeEventsSince rather than the checkpoint read itself,
// so this wrapper is indifferent to how the projector reads and writes its
// checkpoint — it pins the outcome, not the mechanism.
type pgRebuildDuringBatch struct {
	Repository
	rebuild func()
	fired   bool
}

func (r *pgRebuildDuringBatch) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	events, err := r.Repository.ListKnowledgeEventsSince(ctx, afterSeq, limit)
	if err != nil || r.fired {
		return events, err
	}
	r.fired = true
	r.rebuild()
	return events, nil
}

// pgRecordingHandler captures log records so a test can assert that a condition
// was reported at a level an operator will actually see.
type pgRecordingHandler struct {
	records []pgRecordedLog
}

type pgRecordedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func (h *pgRecordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *pgRecordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, pgRecordedLog{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *pgRecordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *pgRecordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *pgRecordingHandler) find(message string) (pgRecordedLog, bool) {
	for _, r := range h.records {
		if r.Message == message {
			return r, true
		}
	}
	return pgRecordedLog{}, false
}

// pgSeedArticleEvent appends one ArticleCreated the projector can fold, and
// returns the sequence the log assigned it.
func pgSeedArticleEvent(ctx context.Context, t *testing.T, db *pgxpool.Pool, tenantID, userID uuid.UUID, n int) int64 {
	t.Helper()

	articleID := uuid.New()
	payload, err := json.Marshal(map[string]string{
		"article_id": articleID.String(),
		"title":      fmt.Sprintf("seeded article %d", n),
		"url":        fmt.Sprintf("https://example.test/article/%d", n),
	})
	require.NoError(t, err)

	// actor_id is bound explicitly: the column is nullable, but every producer
	// goes through AppendKnowledgeEvent, which binds a (possibly empty) string,
	// and scanEvents scans it into a plain string. A NULL here would fail the
	// read rather than the projection, which is not what this test is about.
	var seq int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO knowledge_events
		(occurred_at, tenant_id, user_id, actor_type, actor_id, event_type, aggregate_type, aggregate_id, dedupe_key, payload)
		VALUES ($1, $2, $3, 'user', '', 'ArticleCreated', 'article', $4, $5, $6)
		RETURNING event_seq`,
		pgEventAt.Add(time.Duration(n)*time.Minute), tenantID, userID,
		articleID.String(), fmt.Sprintf("article-created-%d", n), payload,
	).Scan(&seq))
	return seq
}

func pgCountHomeItems(ctx context.Context, t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM knowledge_home_items").Scan(&n))
	return n
}

func pgCheckpointSeq(ctx context.Context, t *testing.T, db *pgxpool.Pool) (int64, bool) {
	t.Helper()
	var seq int64
	err := db.QueryRow(ctx,
		`SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`,
		"knowledge-home-projector").Scan(&seq)
	if err != nil {
		require.ErrorContains(t, err, "no rows")
		return 0, false
	}
	return seq, true
}

// The defect, end to end: a batch that started before a rebuild must not
// overwrite the checkpoint the rebuild reset.
//
// Locking the checkpoint row FOR UPDATE makes a concurrent projector wait; it
// does not make it re-read. An unconditional checkpoint upsert therefore lands
// after the rebuild committed, leaving the read models empty behind a
// checkpoint at the tip — the ~326 silently unprojected articles of
// PM-2026-010. The rebuild's reset must survive, and the next tick must rebuild
// the whole read model from the log.
func TestProjector_InFlightBatchMustNotOverwriteAConcurrentRebuildsCheckpointReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	repo := sovereign_db.NewRepository(db)
	tenantID, userID := uuid.New(), uuid.New()

	// Three articles already folded, so the projector starts the racing batch
	// from a real non-zero checkpoint, as it does in production.
	for i := 1; i <= 3; i++ {
		pgSeedArticleEvent(ctx, t, db, tenantID, userID, i)
	}
	require.NoError(t, NewProjector(repo, slog.New(&pgRecordingHandler{}), Config{}).RunBatch(ctx))

	settled, exists := pgCheckpointSeq(ctx, t, db)
	require.True(t, exists, "precondition: the first tick must leave a checkpoint row")
	require.NotZero(t, settled, "precondition: the racing batch must start from a non-zero checkpoint")
	require.EqualValues(t, 3, pgCountHomeItems(ctx, t, db), "precondition: three articles are projected")

	// Two more articles arrive. This is the batch that races the rebuild.
	var tip int64
	for i := 4; i <= 5; i++ {
		tip = pgSeedArticleEvent(ctx, t, db, tenantID, userID, i)
	}

	target, err := sovereign_db.LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	logs := &pgRecordingHandler{}
	racing := &pgRebuildDuringBatch{Repository: repo, rebuild: func() {
		_, err := repo.RebuildProjection(ctx, target)
		require.NoError(t, err)
	}}
	require.NoError(t, NewProjector(racing, slog.New(logs), Config{}).RunBatch(ctx),
		"losing the checkpoint race is a recoverable outcome, not a batch failure")
	require.True(t, racing.fired, "the rebuild must actually have run inside the batch")

	seq, exists := pgCheckpointSeq(ctx, t, db)
	require.True(t, exists, "the rebuild leaves a checkpoint row behind")
	assert.Zero(t, seq,
		"the rebuild reset the checkpoint to 0; a batch that read %d before the reset must not "+
			"advance it to %d, or events 1..%d are never re-folded into the emptied read models",
		settled, tip, tip)

	rejected, ok := logs.find("knowledge_home_projector.checkpoint_advance_rejected")
	require.True(t, ok, "a rejected checkpoint advance must be reported, not swallowed")
	assert.Equal(t, slog.LevelError, rejected.Level,
		"the read model is mid-rebuild and a batch's work was abandoned; that is an error-level event")
	assert.EqualValues(t, settled, rejected.Attrs["expected_seq"])
	assert.EqualValues(t, tip, rejected.Attrs["attempted_seq"])

	// The whole point of the reset surviving: the very next tick re-folds the
	// entire log, so every article is back in the read model.
	require.NoError(t, NewProjector(repo, slog.New(&pgRecordingHandler{}), Config{}).RunBatch(ctx))
	assert.EqualValues(t, 5, pgCountHomeItems(ctx, t, db),
		"the next tick must re-fold the whole event log into the rebuilt read model")
	seq, _ = pgCheckpointSeq(ctx, t, db)
	assert.Equal(t, tip, seq, "and then the checkpoint catches up to the tip again")
}
