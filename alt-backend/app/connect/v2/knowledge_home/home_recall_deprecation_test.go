package knowledge_home

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/usecase/recall_rail_usecase"
)

// The legacy recall endpoints are still online so existing clients keep
// working, but every invocation now writes a structured deprecation log
// (`legacy.recall_rail.deprecated`). Operators read this log via Grafana to
// confirm zero remaining traffic before the RPC is removed (ADR-000913 §D-9).

func deprecationTestUserContext(t *testing.T) *domain.UserContext {
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

func loggerCapturingTo(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestGetRecallRail_LogsDeprecationWarning(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	handler := &Handler{
		logger: loggerCapturingTo(buf),
		recallRailUsecase: recall_rail_usecase.NewRecallRailUsecase(&mockRecallCandidatesPort{
			candidates: nil,
		}, nil, nil),
	}

	ctx := domain.SetUserContext(context.Background(), deprecationTestUserContext(t))
	_, err := handler.GetRecallRail(ctx, connect.NewRequest(&knowledgehomev1.GetRecallRailRequest{Limit: 5}))
	require.NoError(t, err)

	assertDeprecationLogged(t, buf, "GetRecallRail")
}

func TestTrackRecallAction_LogsDeprecationWarning(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	// recallSnoozeUsecase / recallDismissUsecase remain nil — we expect the
	// deprecation log BEFORE any branch enters, so the invalid argument path
	// is acceptable as long as the log fires first.
	handler := &Handler{logger: loggerCapturingTo(buf)}

	ctx := domain.SetUserContext(context.Background(), deprecationTestUserContext(t))
	// Use an unknown action_type so the switch falls through without
	// invoking any usecase — we only care that the deprecation log fired
	// before validation/dispatch.
	_, _ = handler.TrackRecallAction(ctx, connect.NewRequest(&knowledgehomev1.TrackRecallActionRequest{
		ActionType: "deprecation-log-probe",
		ItemKey:    "article:1",
	}))

	assertDeprecationLogged(t, buf, "TrackRecallAction")
}

// assertDeprecationLogged scans the JSON-line capture for the deprecation
// entry and confirms the rpc tag matches the legacy RPC name.
func assertDeprecationLogged(t *testing.T, buf *bytes.Buffer, wantRPC string) {
	t.Helper()
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["msg"] == "legacy.recall_rail.deprecated" && entry["rpc"] == wantRPC {
			found = true
			break
		}
	}
	require.True(t, found,
		"expected legacy.recall_rail.deprecated log line with rpc=%s, got: %s", wantRPC, buf.String())
}
