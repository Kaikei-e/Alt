package knowledge_projection_health_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGetActiveVersionPort struct {
	version *domain.KnowledgeProjectionVersion
	err     error
}

func (m *mockGetActiveVersionPort) GetActiveVersion(_ context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return m.version, m.err
}

type mockGetCheckpointPort struct {
	seq int64
	err error
}

func (m *mockGetCheckpointPort) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return m.seq, m.err
}

type mockListBackfillJobsPort struct {
	jobs []domain.KnowledgeBackfillJob
	err  error
}

func (m *mockListBackfillJobsPort) ListBackfillJobs(_ context.Context) ([]domain.KnowledgeBackfillJob, error) {
	return m.jobs, m.err
}

func TestGetHealth(t *testing.T) {
	logger.InitLogger()

	t.Run("returns health status", func(t *testing.T) {
		now := time.Now()
		versionPort := &mockGetActiveVersionPort{
			version: &domain.KnowledgeProjectionVersion{
				Version:     1,
				Description: "Initial",
				Status:      "active",
				ActivatedAt: &now,
			},
		}
		checkpointPort := &mockGetCheckpointPort{seq: 42}
		backfillPort := &mockListBackfillJobsPort{
			jobs: []domain.KnowledgeBackfillJob{
				{Status: domain.BackfillStatusCompleted, ProcessedEvents: 100},
			},
		}

		uc := NewUsecase(versionPort, checkpointPort, backfillPort)
		health, err := uc.GetHealth(context.Background())
		require.NoError(t, err)

		assert.Equal(t, 1, health.ActiveVersion)
		assert.Equal(t, int64(42), health.CheckpointSeq)
		assert.Len(t, health.BackfillJobs, 1)
	})

	t.Run("returns partial health on version error", func(t *testing.T) {
		versionPort := &mockGetActiveVersionPort{err: assert.AnError}
		checkpointPort := &mockGetCheckpointPort{seq: 10}
		backfillPort := &mockListBackfillJobsPort{}

		uc := NewUsecase(versionPort, checkpointPort, backfillPort)
		health, err := uc.GetHealth(context.Background())
		require.NoError(t, err)

		assert.Equal(t, 0, health.ActiveVersion)
		assert.Equal(t, int64(10), health.CheckpointSeq)
	})
}
