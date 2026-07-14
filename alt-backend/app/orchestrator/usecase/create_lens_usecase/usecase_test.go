package create_lens_usecase

import (
	"alt/domain"
	"context"
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

func TestCreateLens_Success(t *testing.T) {
	uc := NewCreateLensUsecase(&mockCreateLensPort{}, &mockCreateLensVersionPort{})

	result, err := uc.Execute(context.Background(), CreateLensInput{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Name:     "test",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
}
