package knowledge_home

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/recall_snooze_usecase"
)

type stubSnoozeCandidatePort struct {
	until time.Time
	calls int
}

func (s *stubSnoozeCandidatePort) SnoozeRecallCandidate(_ context.Context, _ uuid.UUID, _ string, until time.Time, _ time.Time) error {
	s.calls++
	s.until = until
	return nil
}

func snoozeTestUserContext(t *testing.T) *domain.UserContext {
	t.Helper()
	return &domain.UserContext{
		UserID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Email:     "tester@example.com",
		Role:      domain.UserRoleUser,
		SessionID: "session-1",
		LoginAt:   time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

// ADR-000913 §D-9: TrackHomeAction now dispatches snooze and dismiss_recall
// to the recall usecases when configured. These tests exercise the
// dispatch table only — the usecase internals are covered by their own
// suites.

func TestTrackHomeAction_Snooze_UnimplementedWhenUsecaseMissing(t *testing.T) {
	t.Parallel()
	// recallSnoozeUsecase is nil — the handler must surface
	// CodeUnimplemented so legacy deployments without the usecase wired
	// in fail fast rather than silently dropping the request.
	handler := &Handler{}
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "snooze",
		ItemKey:    "article:1",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err),
		"snooze with nil usecase must surface as Unimplemented so legacy deployments are detectable")
}

func TestTrackHomeAction_DismissRecall_UnimplementedWhenUsecaseMissing(t *testing.T) {
	t.Parallel()
	handler := &Handler{}
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "dismiss_recall",
		ItemKey:    "article:1",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestTrackHomeAction_ValidatesItemKeyAndActionType(t *testing.T) {
	t.Parallel()
	handler := &Handler{}
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ItemKey: "article:1",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "snooze",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestTrackHomeAction_Snooze_BoundsSnoozeHours(t *testing.T) {
	t.Parallel()
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	t.Run("excessive snooze_hours returns InvalidArgument", func(t *testing.T) {
		candidatePort := &stubSnoozeCandidatePort{}
		eventPort := &mockKnowledgeEventPort{}
		handler := &Handler{
			recallSnoozeUsecase: recall_snooze_usecase.NewRecallSnoozeUsecase(candidatePort, eventPort),
		}
		meta := `{"snooze_hours":1e12}`
		_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
			ActionType:   "snooze",
			ItemKey:      "article:1",
			MetadataJson: &meta,
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		require.Zero(t, candidatePort.calls)
	})

	t.Run("non-positive snooze_hours returns InvalidArgument", func(t *testing.T) {
		candidatePort := &stubSnoozeCandidatePort{}
		eventPort := &mockKnowledgeEventPort{}
		handler := &Handler{
			recallSnoozeUsecase: recall_snooze_usecase.NewRecallSnoozeUsecase(candidatePort, eventPort),
		}
		meta := `{"snooze_hours":0}`
		_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
			ActionType:   "snooze",
			ItemKey:      "article:1",
			MetadataJson: &meta,
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		require.Zero(t, candidatePort.calls)
	})

	t.Run("normal snooze_hours succeeds", func(t *testing.T) {
		candidatePort := &stubSnoozeCandidatePort{}
		eventPort := &mockKnowledgeEventPort{}
		handler := &Handler{
			recallSnoozeUsecase: recall_snooze_usecase.NewRecallSnoozeUsecase(candidatePort, eventPort),
		}
		meta := `{"snooze_hours":24}`
		_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
			ActionType:   "snooze",
			ItemKey:      "article:1",
			MetadataJson: &meta,
		}))
		require.NoError(t, err)
		require.Equal(t, 1, candidatePort.calls)
	})
}

