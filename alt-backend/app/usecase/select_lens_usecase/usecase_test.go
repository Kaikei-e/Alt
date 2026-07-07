package select_lens_usecase

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

func TestSelectLens_Success(t *testing.T) {
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

func TestSelectLens_ClearSuccess(t *testing.T) {
	userID := uuid.New()
	uc := NewSelectLensUsecase(
		&mockGetLensPort{},
		&mockGetCurrentLensVersionPort{},
		&mockSelectCurrentLensPort{},
		&mockClearCurrentLensPort{},
	)

	err := uc.Execute(context.Background(), userID, uuid.Nil)

	require.NoError(t, err)
}

// TestNewSelectLensUsecase_PanicsOnNilClearPort pins CLAUDE.md rule 8: all
// four ports are required composition-root dependencies (knowledge_module.go
// always wires the sovereign client for each), so a nil clearPort means DI
// forgot to wire it. The previous `if u.clearPort == nil { return nil }`
// inside Execute faked a successful lens clear instead of surfacing the
// wiring bug — see .claude/rules/di-wiring.md.
func TestNewSelectLensUsecase_PanicsOnNilClearPort(t *testing.T) {
	assert.Panics(t, func() {
		NewSelectLensUsecase(
			&mockGetLensPort{},
			&mockGetCurrentLensVersionPort{},
			&mockSelectCurrentLensPort{},
			nil,
		)
	})
}
