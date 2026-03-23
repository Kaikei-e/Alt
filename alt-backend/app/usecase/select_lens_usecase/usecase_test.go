package select_lens_usecase

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

type mockGetCurrentLensVersionPort struct {
	version *domain.KnowledgeLensVersion
	err     error
}

func (m *mockGetCurrentLensVersionPort) GetCurrentLensVersion(_ context.Context, _ uuid.UUID) (*domain.KnowledgeLensVersion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.version, nil
}

type mockSelectCurrentLensPort struct{ called bool }

func (m *mockSelectCurrentLensPort) SelectCurrentLens(_ context.Context, _ domain.KnowledgeCurrentLens) error {
	m.called = true
	return nil
}

type mockClearCurrentLensPort struct{ called bool }

func (m *mockClearCurrentLensPort) ClearCurrentLens(_ context.Context, _ uuid.UUID) error {
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

func TestSelectLens_RoutesThroughSovereign_Select(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()
	mock := &mockCurationMutator{}
	uc := NewSelectLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockGetCurrentLensVersionPort{version: &domain.KnowledgeLensVersion{LensVersionID: versionID}},
		&mockSelectCurrentLensPort{},
		&mockClearCurrentLensPort{},
	)
	uc.SetCurationMutator(mock)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
	assert.Equal(t, knowledge_sovereign_port.MutationSelectLens, mock.calls[0])
}

func TestSelectLens_RoutesThroughSovereign_Clear(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	mock := &mockCurationMutator{}
	uc := NewSelectLensUsecase(
		&mockGetLensPort{},
		&mockGetCurrentLensVersionPort{},
		&mockSelectCurrentLensPort{},
		&mockClearCurrentLensPort{},
	)
	uc.SetCurationMutator(mock)

	err := uc.Execute(context.Background(), userID, uuid.Nil)

	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
	assert.Equal(t, knowledge_sovereign_port.MutationClearLens, mock.calls[0])
}

func TestSelectLens_FallsBackWithoutSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()
	uc := NewSelectLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockGetCurrentLensVersionPort{version: &domain.KnowledgeLensVersion{LensVersionID: versionID}},
		&mockSelectCurrentLensPort{},
		&mockClearCurrentLensPort{},
	)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
}

func TestSelectLens_SovereignError_NonFatal(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()
	mock := &mockCurationMutator{err: errors.New("sovereign failure")}
	uc := NewSelectLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockGetCurrentLensVersionPort{version: &domain.KnowledgeLensVersion{LensVersionID: versionID}},
		&mockSelectCurrentLensPort{},
		&mockClearCurrentLensPort{},
	)
	uc.SetCurationMutator(mock)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
}
