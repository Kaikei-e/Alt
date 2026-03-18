package knowledge_audit_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ports ---

type mockCreateProjectionAuditPort struct {
	created *domain.ProjectionAudit
	err     error
}

func (m *mockCreateProjectionAuditPort) CreateProjectionAudit(_ context.Context, audit *domain.ProjectionAudit) error {
	m.created = audit
	return m.err
}

type mockListProjectionAuditsPort struct {
	audits []domain.ProjectionAudit
	err    error
}

func (m *mockListProjectionAuditsPort) ListProjectionAudits(_ context.Context, _ string, _ int) ([]domain.ProjectionAudit, error) {
	return m.audits, m.err
}

// --- tests ---

func TestRunProjectionAudit(t *testing.T) {
	logger.InitLogger()

	t.Run("creates audit record", func(t *testing.T) {
		createPort := &mockCreateProjectionAuditPort{}
		uc := NewUsecase(createPort, nil)

		audit, err := uc.RunProjectionAudit(context.Background(), "knowledge_home", "v1", 100)
		require.NoError(t, err)
		require.NotNil(t, audit)
		assert.Equal(t, "knowledge_home", audit.ProjectionName)
		assert.Equal(t, "v1", audit.ProjectionVersion)
		assert.Equal(t, 100, audit.SampleSize)
		assert.NotEqual(t, uuid.Nil, audit.AuditID)
		assert.False(t, audit.CheckedAt.IsZero())
		assert.NotNil(t, createPort.created)
	})

	t.Run("returns error when create fails", func(t *testing.T) {
		createPort := &mockCreateProjectionAuditPort{err: assert.AnError}
		uc := NewUsecase(createPort, nil)

		_, err := uc.RunProjectionAudit(context.Background(), "knowledge_home", "v1", 50)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create projection audit")
	})

	t.Run("returns error with invalid sample size", func(t *testing.T) {
		createPort := &mockCreateProjectionAuditPort{}
		uc := NewUsecase(createPort, nil)

		_, err := uc.RunProjectionAudit(context.Background(), "knowledge_home", "v1", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sample_size")
	})

	t.Run("returns error with empty projection name", func(t *testing.T) {
		createPort := &mockCreateProjectionAuditPort{}
		uc := NewUsecase(createPort, nil)

		_, err := uc.RunProjectionAudit(context.Background(), "", "v1", 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "projection_name")
	})
}

func TestListProjectionAudits(t *testing.T) {
	logger.InitLogger()

	t.Run("delegates to port", func(t *testing.T) {
		listPort := &mockListProjectionAuditsPort{
			audits: []domain.ProjectionAudit{
				{AuditID: uuid.New(), ProjectionName: "knowledge_home", SampleSize: 100},
				{AuditID: uuid.New(), ProjectionName: "knowledge_home", SampleSize: 50},
			},
		}
		uc := NewUsecase(nil, listPort)

		audits, err := uc.ListProjectionAudits(context.Background(), "knowledge_home", 10)
		require.NoError(t, err)
		assert.Len(t, audits, 2)
	})

	t.Run("returns error on port failure", func(t *testing.T) {
		listPort := &mockListProjectionAuditsPort{err: assert.AnError}
		uc := NewUsecase(nil, listPort)

		_, err := uc.ListProjectionAudits(context.Background(), "knowledge_home", 10)
		require.Error(t, err)
	})
}
