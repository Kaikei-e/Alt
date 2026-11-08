package update_lens_usecase

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

type mockCreateLensVersionPort struct{ called bool }

func (m *mockCreateLensVersionPort) CreateLensVersion(_ context.Context, _ domain.KnowledgeLensVersion) error {
	m.called = true
	return nil
}

func TestUpdateLens_Success(t *testing.T) {
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

func TestUpdateLens_NilAndArchivedLensReturnErrLensNotFound(t *testing.T) {
	userID := uuid.New()
	lensID := uuid.New()
	archivedAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	t.Run("nil lens returns ErrLensNotFound", func(t *testing.T) {
		versionPort := &mockCreateLensVersionPort{}
		uc := NewUpdateLensUsecase(&mockGetLensPort{lens: nil}, versionPort)

		version, err := uc.Execute(context.Background(), UpdateLensInput{
			LensID: lensID,
			UserID: userID,
		})

		require.ErrorIs(t, err, domain.ErrLensNotFound)
		assert.Nil(t, version)
		assert.False(t, versionPort.called)
	})

	t.Run("archived lens returns ErrLensNotFound", func(t *testing.T) {
		versionPort := &mockCreateLensVersionPort{}
		uc := NewUpdateLensUsecase(
			&mockGetLensPort{lens: &domain.KnowledgeLens{LensID: lensID, UserID: userID, ArchivedAt: &archivedAt}},
			versionPort,
		)

		version, err := uc.Execute(context.Background(), UpdateLensInput{
			LensID: lensID,
			UserID: userID,
		})

		require.ErrorIs(t, err, domain.ErrLensNotFound)
		assert.Nil(t, version)
		assert.False(t, versionPort.called)
	})
}
