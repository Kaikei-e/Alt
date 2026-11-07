package knowledge_home

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/archive_lens_usecase"
	"alt/orchestrator/usecase/select_lens_usecase"
	"alt/orchestrator/usecase/update_lens_usecase"
)

type stubGetLensPort struct {
	lens *domain.KnowledgeLens
	err  error
}

func (s *stubGetLensPort) GetLens(_ context.Context, _ uuid.UUID) (*domain.KnowledgeLens, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.lens, nil
}

type stubArchiveLensPort struct {
	called bool
}

func (s *stubArchiveLensPort) ArchiveLens(_ context.Context, _ uuid.UUID) error {
	s.called = true
	return nil
}

type stubCreateLensVersionPort struct {
	called bool
}

func (s *stubCreateLensVersionPort) CreateLensVersion(_ context.Context, _ domain.KnowledgeLensVersion) error {
	s.called = true
	return nil
}

type stubGetCurrentLensVersionPort struct {
	version *domain.KnowledgeLensVersion
}

func (s *stubGetCurrentLensVersionPort) GetCurrentLensVersion(_ context.Context, _ uuid.UUID) (*domain.KnowledgeLensVersion, error) {
	return s.version, nil
}

type stubSelectCurrentLensPort struct {
	called bool
}

func (s *stubSelectCurrentLensPort) SelectCurrentLens(_ context.Context, _ domain.KnowledgeCurrentLens) error {
	s.called = true
	return nil
}

type stubClearCurrentLensPort struct {
	called bool
}

func (s *stubClearCurrentLensPort) ClearCurrentLens(_ context.Context, _ uuid.UUID) error {
	s.called = true
	return nil
}

func testUserContextWithID(userID uuid.UUID) context.Context {
	loginAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	user := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   loginAt,
		ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	return domain.SetUserContext(context.Background(), user)
}

func TestHomeLens_OtherUsersLensReturnsNotFound(t *testing.T) {
	callerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ownerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	lensID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	getLens := &stubGetLensPort{
		lens: &domain.KnowledgeLens{
			LensID: lensID,
			UserID: ownerID,
			Name:   "other-user-lens",
		},
	}
	archivePort := &stubArchiveLensPort{}
	versionPort := &stubCreateLensVersionPort{}
	selectPort := &stubSelectCurrentLensPort{}
	clearPort := &stubClearCurrentLensPort{}

	h := &Handler{
		archiveLensUsecase: archive_lens_usecase.NewArchiveLensUsecase(getLens, archivePort),
		updateLensUsecase:  update_lens_usecase.NewUpdateLensUsecase(getLens, versionPort),
		selectLensUsecase: select_lens_usecase.NewSelectLensUsecase(
			getLens,
			&stubGetCurrentLensVersionPort{version: &domain.KnowledgeLensVersion{LensVersionID: uuid.New()}},
			selectPort,
			clearPort,
		),
		logger: slog.Default(),
	}

	ctx := testUserContextWithID(callerID)

	t.Run("DeleteLens returns CodeNotFound when lens belongs to another user", func(t *testing.T) {
		_, err := h.DeleteLens(ctx, connect.NewRequest(&knowledgehomev1.DeleteLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.False(t, archivePort.called)
	})

	t.Run("UpdateLens returns CodeNotFound when lens belongs to another user", func(t *testing.T) {
		_, err := h.UpdateLens(ctx, connect.NewRequest(&knowledgehomev1.UpdateLensRequest{
			LensId: lensID.String(),
			Version: &knowledgehomev1.LensVersion{
				QueryText: "rust",
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.False(t, versionPort.called)
	})

	t.Run("SelectLens returns CodeNotFound when lens belongs to another user", func(t *testing.T) {
		_, err := h.SelectLens(ctx, connect.NewRequest(&knowledgehomev1.SelectLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.False(t, selectPort.called)
	})
}
