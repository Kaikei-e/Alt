package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// TestRunBatch_DrainsMultipleBatchesUntilLogEmpty asserts ADR-000914 §projector
// performance behaviour: a single RunBatch invocation drains consecutive
// batches up to MaxBatchesPerTick. With BatchSize=2 and 5 events in the log,
// MaxBatchesPerTick=3 lets the first invocation consume all 5 (2 + 2 + 1)
// before the short-batch break short-circuits.
func TestRunBatch_DrainsMultipleBatchesUntilLogEmpty(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	t0 := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	events := make([]sovereign_db.KnowledgeEvent, 0, 5)
	for i := range 5 {
		payload, err := json.Marshal(map[string]any{"entry_key": "drain-entry"})
		require.NoError(t, err)
		events = append(events, sovereign_db.KnowledgeEvent{
			EventID:       uuid.New(),
			EventSeq:      int64(100 + i),
			OccurredAt:    t0.Add(time.Duration(i) * time.Minute),
			TenantID:      tenantID,
			UserID:        &userID,
			EventType:     EventHomeItemsSeen,
			AggregateType: "knowledge_loop_entry",
			AggregateID:   "drain-entry",
			Payload:       payload,
		})
	}
	repo := &fakeRepo{events: events}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{
		BatchSize:         2,
		MaxBatchesPerTick: 3,
	})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Equal(t, int64(104), repo.checkpoint,
		"single RunBatch with MaxBatchesPerTick=3 must drain all 5 events (104 = last seq)")
}

// TestRunBatch_RespectsMaxBatchesPerTickCap covers the opposite side: when the
// log has more events than MaxBatchesPerTick × BatchSize, RunBatch must
// yield back to the caller (so the goroutine scheduler can run other work)
// rather than spinning forever.
func TestRunBatch_RespectsMaxBatchesPerTickCap(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	t0 := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	// 10 events; BatchSize=2 × cap=2 => 4 events consumed before yield.
	events := make([]sovereign_db.KnowledgeEvent, 0, 10)
	for i := range 10 {
		payload, err := json.Marshal(map[string]any{"entry_key": "cap-entry"})
		require.NoError(t, err)
		events = append(events, sovereign_db.KnowledgeEvent{
			EventID:       uuid.New(),
			EventSeq:      int64(200 + i),
			OccurredAt:    t0.Add(time.Duration(i) * time.Minute),
			TenantID:      tenantID,
			UserID:        &userID,
			EventType:     EventHomeItemsSeen,
			AggregateType: "knowledge_loop_entry",
			AggregateID:   "cap-entry",
			Payload:       payload,
		})
	}
	repo := &fakeRepo{events: events}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{
		BatchSize:         2,
		MaxBatchesPerTick: 2,
	})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Equal(t, int64(203), repo.checkpoint,
		"cap=2 with BatchSize=2 must stop after 4 events (last seq = 203), letting the goroutine yield")
}

// TestRunBatch_EmptyLogYieldsImmediately preserves the existing semantic that
// a quiet log does not spin or log a tick-complete record.
func TestRunBatch_EmptyLogYieldsImmediately(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{}
	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{
		BatchSize:         100,
		MaxBatchesPerTick: 4,
	})
	require.NoError(t, p.RunBatch(context.Background()))
	require.Equal(t, int64(0), repo.checkpoint, "empty log must not advance the checkpoint")
}
