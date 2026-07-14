package knowledge_backfill_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCreateBackfillJobPort implements knowledge_backfill_port.CreateBackfillJobPort.
type mockCreateBackfillJobPort struct {
	created *domain.KnowledgeBackfillJob
	err     error
}

func (m *mockCreateBackfillJobPort) CreateBackfillJob(_ context.Context, job domain.KnowledgeBackfillJob) error {
	m.created = &job
	return m.err
}

// mockGetBackfillJobPort implements knowledge_backfill_port.GetBackfillJobPort.
type mockGetBackfillJobPort struct {
	job *domain.KnowledgeBackfillJob
	err error
}

func (m *mockGetBackfillJobPort) GetBackfillJob(_ context.Context, _ uuid.UUID) (*domain.KnowledgeBackfillJob, error) {
	return m.job, m.err
}

// mockUpdateBackfillJobPort implements knowledge_backfill_port.UpdateBackfillJobPort.
type mockUpdateBackfillJobPort struct {
	updated *domain.KnowledgeBackfillJob
	err     error
}

func (m *mockUpdateBackfillJobPort) UpdateBackfillJob(_ context.Context, job domain.KnowledgeBackfillJob) error {
	m.updated = &job
	return m.err
}

// mockListBackfillJobsPort implements knowledge_backfill_port.ListBackfillJobsPort.
type mockListBackfillJobsPort struct {
	jobs []domain.KnowledgeBackfillJob
	err  error
}

func (m *mockListBackfillJobsPort) ListBackfillJobs(_ context.Context) ([]domain.KnowledgeBackfillJob, error) {
	return m.jobs, m.err
}

type mockCountBackfillArticlesPort struct {
	count int
	err   error
}

func (m *mockCountBackfillArticlesPort) CountBackfillArticles(_ context.Context) (int, error) {
	return m.count, m.err
}

// mockAppendKnowledgeEventPort implements knowledge_event_port.AppendKnowledgeEventPort.
type mockAppendKnowledgeEventPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (m *mockAppendKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	m.events = append(m.events, event)
	if m.err != nil {
		return 0, m.err
	}
	return int64(len(m.events)), nil
}

func TestStartBackfill(t *testing.T) {
	logger.InitLogger()

	t.Run("creates pending job", func(t *testing.T) {
		createPort := &mockCreateBackfillJobPort{}
		countPort := &mockCountBackfillArticlesPort{count: 42}
		uc := NewUsecase(createPort, nil, nil, nil, countPort, nil)

		job, err := uc.StartBackfill(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, domain.BackfillStatusPending, job.Status)
		assert.Equal(t, 1, job.ProjectionVersion)
		assert.Equal(t, 42, job.TotalEvents)
		assert.NotEqual(t, uuid.Nil, job.JobID)
		assert.NotNil(t, createPort.created)
	})

	t.Run("returns error when create fails", func(t *testing.T) {
		createPort := &mockCreateBackfillJobPort{err: assert.AnError}
		countPort := &mockCountBackfillArticlesPort{count: 1}
		uc := NewUsecase(createPort, nil, nil, nil, countPort, nil)

		_, err := uc.StartBackfill(context.Background(), 1)
		require.Error(t, err)
	})

	t.Run("returns error when count fails", func(t *testing.T) {
		createPort := &mockCreateBackfillJobPort{}
		countPort := &mockCountBackfillArticlesPort{err: assert.AnError}
		uc := NewUsecase(createPort, nil, nil, nil, countPort, nil)

		_, err := uc.StartBackfill(context.Background(), 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "count backfill articles")
	})
}

func TestPauseBackfill(t *testing.T) {
	logger.InitLogger()

	t.Run("pauses a running job", func(t *testing.T) {
		jobID := uuid.New()
		getPort := &mockGetBackfillJobPort{
			job: &domain.KnowledgeBackfillJob{
				JobID:  jobID,
				Status: domain.BackfillStatusRunning,
			},
		}
		updatePort := &mockUpdateBackfillJobPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil)

		err := uc.PauseBackfill(context.Background(), jobID)
		require.NoError(t, err)
		require.NotNil(t, updatePort.updated)
		assert.Equal(t, domain.BackfillStatusPaused, updatePort.updated.Status)
	})

	t.Run("error when job not running", func(t *testing.T) {
		jobID := uuid.New()
		getPort := &mockGetBackfillJobPort{
			job: &domain.KnowledgeBackfillJob{
				JobID:  jobID,
				Status: domain.BackfillStatusCompleted,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil)

		err := uc.PauseBackfill(context.Background(), jobID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot pause")
	})
}

func TestResumeBackfill(t *testing.T) {
	logger.InitLogger()

	t.Run("resumes a paused job", func(t *testing.T) {
		jobID := uuid.New()
		getPort := &mockGetBackfillJobPort{
			job: &domain.KnowledgeBackfillJob{
				JobID:  jobID,
				Status: domain.BackfillStatusPaused,
			},
		}
		updatePort := &mockUpdateBackfillJobPort{}
		uc := NewUsecase(nil, getPort, updatePort, nil, nil, nil)

		err := uc.ResumeBackfill(context.Background(), jobID)
		require.NoError(t, err)
		require.NotNil(t, updatePort.updated)
		assert.Equal(t, domain.BackfillStatusRunning, updatePort.updated.Status)
	})

	t.Run("error when job not paused", func(t *testing.T) {
		jobID := uuid.New()
		getPort := &mockGetBackfillJobPort{
			job: &domain.KnowledgeBackfillJob{
				JobID:  jobID,
				Status: domain.BackfillStatusRunning,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil)

		err := uc.ResumeBackfill(context.Background(), jobID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot resume")
	})
}

func TestGetBackfillStatus(t *testing.T) {
	logger.InitLogger()

	t.Run("returns job status", func(t *testing.T) {
		jobID := uuid.New()
		now := time.Now()
		getPort := &mockGetBackfillJobPort{
			job: &domain.KnowledgeBackfillJob{
				JobID:           jobID,
				Status:          domain.BackfillStatusRunning,
				TotalEvents:     100,
				ProcessedEvents: 42,
				CreatedAt:       now,
			},
		}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil)

		job, err := uc.GetBackfillStatus(context.Background(), jobID)
		require.NoError(t, err)
		assert.Equal(t, jobID, job.JobID)
		assert.Equal(t, 42, job.ProcessedEvents)
	})

	t.Run("returns error when not found", func(t *testing.T) {
		getPort := &mockGetBackfillJobPort{err: assert.AnError}
		uc := NewUsecase(nil, getPort, nil, nil, nil, nil)

		_, err := uc.GetBackfillStatus(context.Background(), uuid.New())
		require.Error(t, err)
	})
}
