package archive_lens_usecase

import (
	"alt/domain"
	"context"
	"testing"
	"time"

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

func TestArchiveLens_Success(t *testing.T) {
	userID := uuid.New()
	lensID := uuid.New()
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID}},
		&mockArchiveLensPort{},
	)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
}

func TestArchiveLens_NilLensReturnsErrLensNotFound(t *testing.T) {
	archivePort := &mockArchiveLensPort{}
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: nil},
		archivePort,
	)

	err := uc.Execute(context.Background(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, domain.ErrLensNotFound)
	assert.False(t, archivePort.called)
}

func TestArchiveLens_AlreadyArchivedRemainsIdempotent(t *testing.T) {
	userID := uuid.New()
	lensID := uuid.New()
	archivedAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	archivePort := &mockArchiveLensPort{}
	uc := NewArchiveLensUsecase(
		&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID, ArchivedAt: &archivedAt}},
		archivePort,
	)

	err := uc.Execute(context.Background(), userID, lensID)

	require.NoError(t, err)
	assert.True(t, archivePort.called)
}
