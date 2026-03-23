package update_lens_usecase

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

type mockGetLensPort struct {
	lens *domain.KnowledgeLens
	err  error
}

func (m *mockGetLensPort) GetLens(_ context.Context, _ uuid.UUID) (*domain.KnowledgeLens, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.lens, nil
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

func TestUpdateLens_RoutesThroughSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	mock := &mockCurationMutator{}
	uc := NewUpdateLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockCreateLensVersionPort{},
	)
	uc.SetCurationMutator(mock)

	version, err := uc.Execute(context.Background(), UpdateLensInput{
		LensID: lensID,
		UserID: userID,
		Name:   "updated",
	})

	require.NoError(t, err)
	assert.NotNil(t, version)
	assert.Len(t, mock.calls, 1)
	assert.Equal(t, knowledge_sovereign_port.MutationCreateLensVersion, mock.calls[0])
}

func TestUpdateLens_FallsBackWithoutSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	uc := NewUpdateLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockCreateLensVersionPort{},
	)

	version, err := uc.Execute(context.Background(), UpdateLensInput{
		LensID: lensID,
		UserID: userID,
		Name:   "updated",
	})

	require.NoError(t, err)
	assert.NotNil(t, version)
}

func TestUpdateLens_SovereignError_NonFatal(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	mock := &mockCurationMutator{err: errors.New("sovereign failure")}
	uc := NewUpdateLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockCreateLensVersionPort{},
	)
	uc.SetCurationMutator(mock)

	version, err := uc.Execute(context.Background(), UpdateLensInput{
		LensID: lensID,
		UserID: userID,
		Name:   "updated",
	})

	require.NoError(t, err)
	assert.NotNil(t, version)
}
