package archive_lens_usecase

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

type mockArchiveLensPort struct{ called bool }

func (m *mockArchiveLensPort) ArchiveLens(_ context.Context, _ uuid.UUID) error {
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

func TestArchiveLens_RoutesThroughSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	mock := &mockCurationMutator{}
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockArchiveLensPort{},
	)
	uc.SetCurationMutator(mock)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
	assert.Equal(t, knowledge_sovereign_port.MutationArchiveLens, mock.calls[0])
}

func TestArchiveLens_FallsBackWithoutSovereign(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockArchiveLensPort{},
	)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
}

func TestArchiveLens_SovereignError_NonFatal(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	lensID := uuid.New()
	mock := &mockCurationMutator{err: errors.New("sovereign failure")}
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockArchiveLensPort{},
	)
	uc.SetCurationMutator(mock)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
}
