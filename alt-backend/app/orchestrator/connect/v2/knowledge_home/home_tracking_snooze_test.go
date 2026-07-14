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
)

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
