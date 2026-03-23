package create_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCreateLensPort struct{ called bool }

func (m *mockCreateLensPort) CreateLens(_ context.Context, _ domain.KnowledgeLens) error {
	m.called = true
	return nil
}

type mockCreateLensVersionPort struct{ called bool }

func (m *mockCreateLensVersionPort) CreateLensVersion(_ context.Context, _ domain.KnowledgeLensVersion) error {
	m.called = true
	return nil
}

type mockCurationMutator struct {
	calls []string
	err   error
}

func (m *mockCurationMutator) ApplyCurationMutation(_ context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation.MutationType)
	return nil
}

func TestCreateLens_RoutesThroughSovereign(t *testing.T) {
	logger.InitLogger()

	mock := &mockCurationMutator{}
	uc := NewCreateLensUsecase(&mockCreateLensPort{}, &mockCreateLensVersionPort{})
	uc.SetCurationMutator(mock)

	result, err := uc.Execute(context.Background(), CreateLensInput{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Name:     "test",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, mock.calls, 2)
	assert.Equal(t, knowledge_sovereign_port.MutationCreateLens, mock.calls[0])
	assert.Equal(t, knowledge_sovereign_port.MutationCreateLensVersion, mock.calls[1])
}

func TestCreateLens_FallsBackWithoutSovereign(t *testing.T) {
	logger.InitLogger()

	uc := NewCreateLensUsecase(&mockCreateLensPort{}, &mockCreateLensVersionPort{})

	result, err := uc.Execute(context.Background(), CreateLensInput{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Name:     "test",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCreateLens_SovereignError_NonFatal(t *testing.T) {
	logger.InitLogger()

	mock := &mockCurationMutator{err: errors.New("sovereign failure")}
	uc := NewCreateLensUsecase(&mockCreateLensPort{}, &mockCreateLensVersionPort{})
	uc.SetCurationMutator(mock)

	result, err := uc.Execute(context.Background(), CreateLensInput{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Name:     "test",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
}
