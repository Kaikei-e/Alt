package update_lens_usecase

import (
	"alt/domain"
	"context"
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
