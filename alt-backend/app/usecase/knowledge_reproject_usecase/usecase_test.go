package knowledge_reproject_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ports ---

type mockCreateReprojectRunPort struct {
	created *domain.ReprojectRun
	err     error
}

func (m *mockCreateReprojectRunPort) CreateReprojectRun(_ context.Context, run *domain.ReprojectRun) error {
	m.created = run
	return m.err
}

type mockGetReprojectRunPort struct {
	run *domain.ReprojectRun
	err error
}

func (m *mockGetReprojectRunPort) GetReprojectRun(_ context.Context, _ uuid.UUID) (*domain.ReprojectRun, error) {
	return m.run, m.err
}

type mockUpdateReprojectRunPort struct {
	updated *domain.ReprojectRun
	err     error
}

func (m *mockUpdateReprojectRunPort) UpdateReprojectRun(_ context.Context, run *domain.ReprojectRun) error {
	m.updated = run
	return m.err
}

type mockListReprojectRunsPort struct {
	runs []domain.ReprojectRun
	err  error
}

func (m *mockListReprojectRunsPort) ListReprojectRuns(_ context.Context, _ string, _ int) ([]domain.ReprojectRun, error) {
	return m.runs, m.err
}

type mockCompareProjectionsPort struct {
	diff *domain.ReprojectDiffSummary
	err  error
}

func (m *mockCompareProjectionsPort) CompareProjections(_ context.Context, _, _ string) (*domain.ReprojectDiffSummary, error) {
	return m.diff, m.err
}

type mockGetActiveVersionPort struct {
	version *domain.KnowledgeProjectionVersion
	err     error
}

func (m *mockGetActiveVersionPort) GetActiveVersion(_ context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return m.version, m.err
}

type mockActivateVersionPort struct {
	activated int
	err       error
}

func (m *mockActivateVersionPort) ActivateVersion(_ context.Context, version int) error {
	m.activated = version
	return m.err
}

type mockUpdateCheckpointPort struct {
	projectorName string
	lastEventSeq  int64
	err           error
}

func (m *mockUpdateCheckpointPort) UpdateProjectionCheckpoint(_ context.Context, projectorName string, lastSeq int64) error {
	m.projectorName = projectorName
	m.lastEventSeq = lastSeq
	return m.err
}

// --- tests ---

func TestStartReproject(t *testing.T) {
	logger.InitLogger()

	t.Run("creates pending run with valid mode", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		run, err := uc.StartReproject(context.Background(), domain.ReprojectModeFull, "v1", "v2", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ReprojectStatusPending, run.Status)
		assert.Equal(t, domain.ReprojectModeFull, run.Mode)
		assert.Equal(t, "v1", run.FromVersion)
		assert.Equal(t, "v2", run.ToVersion)
		assert.NotEqual(t, uuid.Nil, run.ReprojectRunID)
		assert.NotNil(t, createPort.created)
	})

	t.Run("returns error with invalid mode", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		_, err := uc.StartReproject(context.Background(), "invalid_mode", "v1", "v2", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid reproject mode")
		assert.Nil(t, createPort.created)
	})

	t.Run("time_range mode requires range_start and range_end", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		_, err := uc.StartReproject(context.Background(), domain.ReprojectModeTimeRange, "v1", "v2", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "range_start and range_end")
		assert.Nil(t, createPort.created)
	})

	t.Run("time_range mode succeeds with range provided", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		start := time.Now().Add(-24 * time.Hour)
		end := time.Now()
		run, err := uc.StartReproject(context.Background(), domain.ReprojectModeTimeRange, "v1", "v2", &start, &end)
		require.NoError(t, err)
		assert.Equal(t, domain.ReprojectModeTimeRange, run.Mode)
		assert.NotNil(t, run.RangeStart)
		assert.NotNil(t, run.RangeEnd)
	})

	t.Run("returns error when create port fails", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{err: assert.AnError}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		_, err := uc.StartReproject(context.Background(), domain.ReprojectModeFull, "v1", "v2", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create reproject run")
	})

	// Regression guard: knowledge_reproject_runs.{checkpoint_payload, stats_json,
	// diff_summary_json} are NOT NULL JSONB with DEFAULT '{}'. PostgreSQL only
	// applies DEFAULT when a column is omitted from the INSERT — passing a nil
	// json.RawMessage explicitly sends NULL and the constraint kicks. The
	// usecase therefore has to seed empty-object JSON into all three fields,
	// otherwise the create port fans out a 500 from the DB driver. Live
	// production hit this on /admin/knowledge-home Start Reproject (502).
	t.Run("seeds empty-object JSON into checkpoint_payload / stats_json / diff_summary_json", func(t *testing.T) {
		createPort := &mockCreateReprojectRunPort{}
		uc := NewUsecase(createPort, nil, nil, nil, nil, nil, nil)

		_, err := uc.StartReproject(context.Background(), domain.ReprojectModeFull, "v3", "v4", nil, nil)
		require.NoError(t, err)
		require.NotNil(t, createPort.created)

		for label, raw := range map[string]json.RawMessage{
			"CheckpointPayload": createPort.created.CheckpointPayload,
			"StatsJSON":         createPort.created.StatsJSON,
			"DiffSummaryJSON":   createPort.created.DiffSummaryJSON,
		} {
			require.NotEmptyf(t, raw, "%s must not be nil — would NULL-violate the NOT NULL JSONB column", label)
			assert.JSONEqf(t, "{}", string(raw), "%s must be canonical empty JSON object", label)
		}
	})
}

func TestGetReprojectStatus(t *testing.T) {
	logger.InitLogger()

	t.Run("returns run by ID", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusRunning,
				Mode:           domain.ReprojectModeFull,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil, nil)

		run, err := uc.GetReprojectStatus(context.Background(), runID)
		require.NoError(t, err)
		assert.Equal(t, runID, run.ReprojectRunID)
		assert.Equal(t, domain.ReprojectStatusRunning, run.Status)
	})

	t.Run("returns error when not found", func(t *testing.T) {
		getPort := &mockGetReprojectRunPort{err: assert.AnError}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil, nil)

		_, err := uc.GetReprojectStatus(context.Background(), uuid.New())
		require.Error(t, err)
	})
}

func TestListReprojectRuns(t *testing.T) {
	logger.InitLogger()

	t.Run("delegates to port", func(t *testing.T) {
		listPort := &mockListReprojectRunsPort{
			runs: []domain.ReprojectRun{
				{ReprojectRunID: uuid.New(), Status: domain.ReprojectStatusPending},
				{ReprojectRunID: uuid.New(), Status: domain.ReprojectStatusSwapped},
			},
		}
		uc := NewUsecase(nil, nil, nil, listPort, nil, nil, nil)

		runs, err := uc.ListReprojectRuns(context.Background(), "", 10)
		require.NoError(t, err)
		assert.Len(t, runs, 2)
	})

	t.Run("returns error on port failure", func(t *testing.T) {
		listPort := &mockListReprojectRunsPort{err: assert.AnError}
		uc := NewUsecase(nil, nil, nil, listPort, nil, nil, nil)

		_, err := uc.ListReprojectRuns(context.Background(), "", 10)
		require.Error(t, err)
	})
}

func TestCompareReproject(t *testing.T) {
	logger.InitLogger()

	t.Run("compares validating run", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusValidating,
				FromVersion:    "v1",
				ToVersion:      "v2",
			},
		}
		diff := &domain.ReprojectDiffSummary{
			FromItemCount: 100,
			ToItemCount:   110,
		}
		comparePort := &mockCompareProjectionsPort{diff: diff}
		updatePort := &mockUpdateReprojectRunPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, comparePort, nil, nil)

		result, err := uc.CompareReproject(context.Background(), runID)
		require.NoError(t, err)
		assert.Equal(t, int64(100), result.FromItemCount)
		assert.Equal(t, int64(110), result.ToItemCount)
		// Verify run was updated to swappable
		require.NotNil(t, updatePort.updated)
		assert.Equal(t, domain.ReprojectStatusSwappable, updatePort.updated.Status)
		// Verify diff summary was stored in run
		assert.NotNil(t, updatePort.updated.DiffSummaryJSON)
	})

	t.Run("compares swappable run", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwappable,
				FromVersion:    "v1",
				ToVersion:      "v2",
			},
		}
		diff := &domain.ReprojectDiffSummary{FromItemCount: 50, ToItemCount: 55}
		comparePort := &mockCompareProjectionsPort{diff: diff}
		updatePort := &mockUpdateReprojectRunPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, comparePort, nil, nil)

		result, err := uc.CompareReproject(context.Background(), runID)
		require.NoError(t, err)
		assert.Equal(t, int64(50), result.FromItemCount)
	})

	t.Run("rejects non-validating/swappable run", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusRunning,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil, nil)

		_, err := uc.CompareReproject(context.Background(), runID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot compare")
	})
}

func TestSwapReproject(t *testing.T) {
	logger.InitLogger()

	t.Run("swaps swappable run and activates version", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwappable,
				ToVersion:      "v2",
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)

		err := uc.SwapReproject(context.Background(), runID)
		require.NoError(t, err)
		// ActivateVersion must be called with parsed version number
		assert.Equal(t, 2, activatePort.activated)
		require.NotNil(t, updatePort.updated)
		assert.Equal(t, domain.ReprojectStatusSwapped, updatePort.updated.Status)
		assert.NotNil(t, updatePort.updated.FinishedAt)
	})

	t.Run("does not swap when ActivateVersion fails", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwappable,
				ToVersion:      "v2",
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{err: assert.AnError}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)

		err := uc.SwapReproject(context.Background(), runID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activate version")
		// Run should NOT have been updated to swapped
		assert.Nil(t, updatePort.updated)
	})

	t.Run("fails with invalid version format", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwappable,
				ToVersion:      "invalid",
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)

		err := uc.SwapReproject(context.Background(), runID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse version")
		assert.Equal(t, 0, activatePort.activated)
		assert.Nil(t, updatePort.updated)
	})

	t.Run("rejects non-swappable run", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusValidating,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil, nil)

		err := uc.SwapReproject(context.Background(), runID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot swap")
	})

	t.Run("resets checkpoint to reproject checkpoint on swap", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID:    runID,
				Status:            domain.ReprojectStatusSwappable,
				ToVersion:         "v3",
				CheckpointPayload: json.RawMessage(`{"last_event_seq": 1116081}`),
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{}
		checkpointPort := &mockUpdateCheckpointPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)
		uc.updateCheckpointPort = checkpointPort

		err := uc.SwapReproject(context.Background(), runID)
		require.NoError(t, err)
		assert.Equal(t, 3, activatePort.activated)
		assert.Equal(t, "knowledge-home-projector", checkpointPort.projectorName)
		assert.Equal(t, int64(1116081), checkpointPort.lastEventSeq)
	})

	t.Run("succeeds swap even with empty checkpoint payload", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID:    runID,
				Status:            domain.ReprojectStatusSwappable,
				ToVersion:         "v2",
				CheckpointPayload: nil,
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{}
		checkpointPort := &mockUpdateCheckpointPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)
		uc.updateCheckpointPort = checkpointPort

		err := uc.SwapReproject(context.Background(), runID)
		require.NoError(t, err)
		assert.Equal(t, 2, activatePort.activated)
		// No checkpoint reset when payload is nil
		assert.Equal(t, int64(0), checkpointPort.lastEventSeq)
	})
}

func TestRollbackReproject(t *testing.T) {
	logger.InitLogger()

	t.Run("rolls back swapped run and reverts version", func(t *testing.T) {
		runID := uuid.New()
		now := time.Now()
		statsJSON, _ := json.Marshal(domain.ReprojectStats{EventsProcessed: 100})
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwapped,
				FromVersion:    "v1",
				ToVersion:      "v2",
				FinishedAt:     &now,
				StatsJSON:      statsJSON,
			},
		}
		updatePort := &mockUpdateReprojectRunPort{}
		activatePort := &mockActivateVersionPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil, activatePort)

		err := uc.RollbackReproject(context.Background(), runID)
		require.NoError(t, err)
		require.NotNil(t, updatePort.updated)
		assert.Equal(t, domain.ReprojectStatusCancelled, updatePort.updated.Status)
		assert.Equal(t, 1, activatePort.activated, "should revert to FromVersion v1")
	})

	t.Run("rejects non-swapped run", func(t *testing.T) {
		runID := uuid.New()
		getPort := &mockGetReprojectRunPort{
			run: &domain.ReprojectRun{
				ReprojectRunID: runID,
				Status:         domain.ReprojectStatusSwappable,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil, nil)

		err := uc.RollbackReproject(context.Background(), runID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot rollback")
	})
}
