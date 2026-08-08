package knowledge_home

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/track_home_action_usecase"
)

// stubUserEventPort stands in for the sovereign client at the port boundary
// so the handler test carries the real driver wrap ("sovereign
// AppendKnowledgeUserEvent: %w") without needing a live upstream.
type stubUserEventPort struct {
	err error
}

func (s stubUserEventPort) AppendKnowledgeUserEvent(context.Context, domain.KnowledgeUserEvent) error {
	return s.err
}

type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) at(level slog.Level) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == level {
			out = append(out, r)
		}
	}
	return out
}

func recordText(r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
		return true
	})
	return b.String()
}

func hasAttr(r slog.Record, key string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
			return false
		}
		return true
	})
	return found
}

// sovereignError reproduces the exact production wrap chain:
// knowledge-sovereign answers invalid_argument, sovereign_client wraps it,
// track_home_action_usecase wraps that again.
func sovereignError(code connect.Code, message string) error {
	return fmt.Errorf("sovereign AppendKnowledgeUserEvent: %w",
		connect.NewError(code, errors.New(message)))
}

func trackActionHandler(t *testing.T, upstreamErr error) (*Handler, *recordingHandler) {
	t.Helper()
	capture := &recordingHandler{}
	usecase := track_home_action_usecase.NewTrackHomeActionUsecase(
		stubUserEventPort{err: upstreamErr},
		nil, nil, nil, nil, nil, nil,
	)
	return &Handler{
		trackActionUsecase: usecase,
		logger:             slog.New(capture),
	}, capture
}

// TestTrackHomeAction_SovereignInvalidArgumentIsNotInternal reproduces the
// production incident: knowledge-sovereign rejected the request with
// invalid_argument ("dedupe_key is required") and alt-backend reported a 500
// to its own caller. A 500 sends operators hunting for an outage that did not
// happen, and the BFF's shared non-critical circuit breaker counts it as a
// dependency failure — five clicks blacked out unrelated Knowledge Home reads.
func TestTrackHomeAction_SovereignInvalidArgumentIsNotInternal(t *testing.T) {
	t.Parallel()

	handler, capture := trackActionHandler(t, sovereignError(connect.CodeInvalidArgument, "dedupe_key is required"))
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "open",
		ItemKey:    "article:1",
	}))

	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err),
		"a caller-side upstream fault must not be reported as CodeInternal")

	// Not routed through HandleInternalError: that path is the one that emits
	// an ERROR record carrying an error_id.
	for _, rec := range capture.at(slog.LevelError) {
		require.False(t, hasAttr(rec, "error_id"),
			"caller-side fault must not go through the internal-error path: %s", recordText(rec))
	}

	warns := capture.at(slog.LevelWarn)
	require.NotEmpty(t, warns, "a caller-side fault must still be loud enough to find")

	var found bool
	for _, rec := range warns {
		text := recordText(rec)
		if strings.Contains(text, "dedupe_key is required") &&
			strings.Contains(text, connect.CodeInvalidArgument.String()) &&
			strings.Contains(text, "TrackHomeAction") {
			found = true
		}
	}
	require.True(t, found,
		"WARN must carry operation, upstream code and the verbatim upstream message so the caller bug stays findable")
}

// TestTrackHomeAction_SovereignInternalStaysInternal is the other half of the
// contract: making unknown or genuinely internal upstream faults quietly
// non-5xx would swap one dishonesty for another (Critical Rule 8).
func TestTrackHomeAction_SovereignInternalStaysInternal(t *testing.T) {
	t.Parallel()

	handler, capture := trackActionHandler(t, sovereignError(connect.CodeInternal, "projection store unreachable"))
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "open",
		ItemKey:    "article:1",
	}))

	require.Error(t, err)
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	require.Empty(t, capture.at(slog.LevelWarn),
		"a genuine internal fault must not be downgraded to WARN")

	errRecords := capture.at(slog.LevelError)
	require.Len(t, errRecords, 1)
	require.True(t, hasAttr(errRecords[0], "error_id"),
		"internal faults must keep their error_id for correlation")
}

// TestTrackHomeAction_LocalValidationErrorStaysInternal pins that the new
// mapping keys off the upstream Connect code, not off error text: a plain
// usecase-level error has no code and must stay 5xx.
func TestTrackHomeAction_LocalErrorStaysInternal(t *testing.T) {
	t.Parallel()

	handler, capture := trackActionHandler(t, errors.New("dedupe_key is required"))
	ctx := domain.SetUserContext(context.Background(), snoozeTestUserContext(t))

	_, err := handler.TrackHomeAction(ctx, connect.NewRequest(&knowledgehomev1.TrackHomeActionRequest{
		ActionType: "open",
		ItemKey:    "article:1",
	}))

	require.Error(t, err)
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err),
		"an uncoded error must stay 5xx even when its text looks like a validation failure")
	require.Empty(t, capture.at(slog.LevelWarn))
}
