package reading_projector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestNewProjector_PanicsWhenRepositoryIsNil(t *testing.T) {
	assert.Panics(t, func() {
		NewProjector(nil, nil, Config{})
	}, "Rule 8: nil repository must panic at construction")
}

func TestFakeReadingRepo_CASChecksExistsAndUpdatedAt(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("first-boot absent uncontested advances and stamps Exists + UpdatedAt", func(t *testing.T) {
		repo := newFakeReadingRepo(nil)
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)
		require.False(t, from.Exists)

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.True(t, applied)
		assert.True(t, repo.checkpoint.Exists)
		assert.EqualValues(t, 900, repo.checkpoint.LastEventSeq)
		assert.False(t, repo.checkpoint.UpdatedAt.IsZero(), "applied advance must stamp non-zero UpdatedAt witness")
	})

	t.Run("first-boot absent token rejected when rebuild already created Present(0)", func(t *testing.T) {
		repo := newFakeReadingRepo(nil)
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)
		require.False(t, from.Exists)

		// Concurrent rebuild inserts row with seq=0, Exists=true.
		repo.checkpoint = sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 0,
			UpdatedAt:    t0.Add(time.Minute),
			Exists:       true,
		}

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.False(t, applied, "Absent token must be rejected when row now exists with seq 0")
		assert.Zero(t, repo.checkpoint.LastEventSeq)
		assert.True(t, repo.checkpoint.Exists)
	})

	t.Run("double rebuild 0 -> 0 ABA rejected by UpdatedAt witness", func(t *testing.T) {
		repo := newFakeReadingRepo(nil)
		repo.checkpoint = sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 0,
			UpdatedAt:    t0,
			Exists:       true,
		}
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)
		require.True(t, from.Exists)
		require.Zero(t, from.LastEventSeq)

		// Second rebuild resets 0 -> 0 with a strictly newer updated_at.
		repo.checkpoint = sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 0,
			UpdatedAt:    t0.Add(time.Minute),
			Exists:       true,
		}

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 900)
		require.NoError(t, err)
		assert.False(t, applied, "0 -> 0 ABA must be rejected when UpdatedAt moved")
		assert.Zero(t, repo.checkpoint.LastEventSeq)
	})

	t.Run("consumed token cannot be reused after a successful advance", func(t *testing.T) {
		repo := newFakeReadingRepo(nil)
		repo.checkpoint = sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 100,
			UpdatedAt:    t0,
			Exists:       true,
		}
		from, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		require.NoError(t, err)

		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 100)
		require.NoError(t, err)
		require.True(t, applied)

		// Even when seq stayed 100, UpdatedAt moved so the consumed token is dead.
		reused, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, from, 200)
		require.NoError(t, err)
		assert.False(t, reused, "consumed token must be rejected because UpdatedAt advanced")
		assert.EqualValues(t, 100, repo.checkpoint.LastEventSeq)
	})

	t.Run("present token rejected against absent row", func(t *testing.T) {
		repo := newFakeReadingRepo(nil)
		fakeTok := sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 0,
			UpdatedAt:    t0,
			Exists:       true,
		}
		applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, fakeTok, 100)
		require.NoError(t, err)
		assert.False(t, applied)
		assert.False(t, repo.checkpoint.Exists)
	})
}

func TestRunBatch_RejectsZeroToZeroABAAndFirstBootRebuildRace(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

	t.Run("first-boot race stops tick and logs rejection", func(t *testing.T) {
		repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
			queuedReadingEvt(t, 1, t0, tenantID, userID, uuid.New()),
			queuedReadingEvt(t, 2, t0, tenantID, userID, uuid.New()),
		})
		// Simulate rebuild creating Present(0, rebuild_ts) after projector read Absent
		// and before AdvanceProjectionCheckpointIfUnchanged executes.
		rebuilt := false
		repo.beforeAdvance = func(_ sovereign_db.ProjectionCheckpoint, _ int64) bool {
			if !rebuilt {
				rebuilt = true
				repo.checkpoint = sovereign_db.ProjectionCheckpoint{
					LastEventSeq: 0,
					UpdatedAt:    t0.Add(time.Minute),
					Exists:       true,
				}
			}
			return true
		}

		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{BatchSize: 1, MaxBatchesPerTick: 4})
		require.NoError(t, p.RunBatch(context.Background()))

		assert.Zero(t, repo.checkpoint.LastEventSeq, "first-boot Absent batch must not overwrite rebuild's Present(0)")
		assert.Equal(t, 1, logs.count("reading_projector.checkpoint_advance_rejected"))
	})

	t.Run("0 -> 0 ABA race stops tick and logs rejection", func(t *testing.T) {
		repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
			queuedReadingEvt(t, 1, t0, tenantID, userID, uuid.New()),
		})
		repo.checkpoint = sovereign_db.ProjectionCheckpoint{
			LastEventSeq: 0,
			UpdatedAt:    t0,
			Exists:       true,
		}
		rebuilt := false
		repo.beforeAdvance = func(_ sovereign_db.ProjectionCheckpoint, _ int64) bool {
			if !rebuilt {
				rebuilt = true
				repo.checkpoint = sovereign_db.ProjectionCheckpoint{
					LastEventSeq: 0,
					UpdatedAt:    t0.Add(time.Minute),
					Exists:       true,
				}
			}
			return true
		}

		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{})
		require.NoError(t, p.RunBatch(context.Background()))

		assert.Zero(t, repo.checkpoint.LastEventSeq, "0 -> 0 ABA must not overwrite second rebuild's checkpoint")
		assert.Equal(t, 1, logs.count("reading_projector.checkpoint_advance_rejected"))
	})
}
