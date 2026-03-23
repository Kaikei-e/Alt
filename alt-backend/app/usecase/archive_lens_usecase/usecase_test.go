package archive_lens_usecase

import (
	"alt/domain"
	"context"
	"testing"

	"github.com/google/uuid"
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
