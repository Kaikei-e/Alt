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

type mockGetFreshnessPort struct {
	updatedAt *time.Time
	err       error
}

func (m *mockGetFreshnessPort) GetProjectionFreshness(_ context.Context, _ string) (*time.Time, error) {
	return m.updatedAt, m.err
}

func TestGetHealth(t *testing.T) {
	logger.InitLogger()

	t.Run("returns health status with checkpoint updated_at", func(t *testing.T) {
		now := time.Now()
		checkpointUpdated := now.Add(-2 * time.Minute)
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
		freshnessPort := &mockGetFreshnessPort{updatedAt: &checkpointUpdated}

		uc := NewUsecase(versionPort, checkpointPort, backfillPort, freshnessPort)
		health, err := uc.GetHealth(context.Background())
		require.NoError(t, err)

		assert.Equal(t, 1, health.ActiveVersion)
		assert.Equal(t, int64(42), health.CheckpointSeq)
		assert.Len(t, health.BackfillJobs, 1)
		assert.WithinDuration(t, checkpointUpdated, health.LastUpdated, time.Second)
	})

	t.Run("returns partial health on version error", func(t *testing.T) {
		versionPort := &mockGetActiveVersionPort{err: assert.AnError}
		checkpointPort := &mockGetCheckpointPort{seq: 10}
		backfillPort := &mockListBackfillJobsPort{}
		freshnessPort := &mockGetFreshnessPort{err: assert.AnError}

		uc := NewUsecase(versionPort, checkpointPort, backfillPort, freshnessPort)
		health, err := uc.GetHealth(context.Background())
		require.NoError(t, err)

		assert.Equal(t, 0, health.ActiveVersion)
		assert.Equal(t, int64(10), health.CheckpointSeq)
		// LastUpdated falls back to time.Now() when freshness fails
		assert.WithinDuration(t, time.Now(), health.LastUpdated, 2*time.Second)
	})

	t.Run("returns time.Now when freshness returns nil", func(t *testing.T) {
		versionPort := &mockGetActiveVersionPort{}
		checkpointPort := &mockGetCheckpointPort{seq: 5}
		backfillPort := &mockListBackfillJobsPort{}
		freshnessPort := &mockGetFreshnessPort{updatedAt: nil}

		uc := NewUsecase(versionPort, checkpointPort, backfillPort, freshnessPort)
		health, err := uc.GetHealth(context.Background())
		require.NoError(t, err)

		assert.WithinDuration(t, time.Now(), health.LastUpdated, 2*time.Second)
	})
}

func TestResolveLastUpdated(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	past := now.Add(-10 * time.Minute)

	tests := []struct {
		name      string
		updatedAt *time.Time
		fallback  time.Time
		expected  time.Time
	}{
		{
			name:      "uses non-nil updatedAt",
			updatedAt: &past,
			fallback:  now,
			expected:  past,
		},
		{
			name:      "falls back to now when nil",
			updatedAt: nil,
			fallback:  now,
			expected:  now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLastUpdated(tt.updatedAt, tt.fallback)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildHealthStatus(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	jobs := []domain.KnowledgeBackfillJob{
		{Status: domain.BackfillStatusCompleted, ProcessedEvents: 100},
	}

	t.Run("builds status with active version", func(t *testing.T) {
		res := buildHealthStatus(3, 42, jobs, now)
		assert.Equal(t, 3, res.ActiveVersion)
		assert.Equal(t, int64(42), res.CheckpointSeq)
		assert.Equal(t, jobs, res.BackfillJobs)
		assert.Equal(t, now, res.LastUpdated)
	})

	t.Run("builds status with zero version", func(t *testing.T) {
		res := buildHealthStatus(0, 15, nil, now)
		assert.Equal(t, 0, res.ActiveVersion)
		assert.Equal(t, int64(15), res.CheckpointSeq)
		assert.Nil(t, res.BackfillJobs)
		assert.Equal(t, now, res.LastUpdated)
	})
}
