package knowledge_home

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"
	"alt/orchestrator/usecase/recall_rail_usecase"
	"alt/utils/logger"
)

// mockEventsForUserPort implements knowledge_event_port.ListKnowledgeEventsForUserPort.
type mockEventsForUserPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (m *mockEventsForUserPort) ListKnowledgeEventsSinceForUser(_ context.Context, _, _ uuid.UUID, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	return m.events, m.err
}

// newStreamTestHandler creates a handler with eventsPort and featureFlags for stream tests.
func newStreamTestHandler(flagPort *mockFeatureFlagPort, eventsPort *mockListEventsPort) *Handler {
	handler := NewHandler(
		nil, nil, nil, // home, seen, action
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens: create, update, list, select, archive
		eventsPort,
		nil, // eventsForUserPort
		nil, // lensVisibilityPort
		nil, // resolveLensPort
		flagPort,
		nil, // metrics
		slog.Default(),
	)
	return handler
}

// StreamKnowledgeHomeUpdates receives *connect.ServerStream which is a concrete type
// that cannot be instantiated or mocked in pure unit tests. We test the flag check
// through the extracted streamFlagGuard helper.
func TestStreamKnowledgeHomeUpdates_FeatureFlagDisabled(t *testing.T) {
	logger.InitLogger()
	flagPort := &mockFeatureFlagPort{
		enabledFlags: map[string]bool{
			domain.FlagStreamUpdates: false,
		},
	}
	eventsPort := &mockListEventsPort{}
	handler := newStreamTestHandler(flagPort, eventsPort)

	ctx := testUserContext()
	err := handler.streamFlagGuard(ctx)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

func TestStreamKnowledgeHomeUpdates_Unauthenticated(t *testing.T) {
	logger.InitLogger()
	handler := newStreamTestHandler(nil, nil)

	err := handler.streamFlagGuard(context.Background())
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestHandler_StreamKnowledgeHomeUpdates_InvalidLensID(t *testing.T) {
	logger.InitLogger()
	handler := newStreamTestHandler(nil, nil)

	req := connect.NewRequest(&knowledgehomev1.StreamKnowledgeHomeUpdatesRequest{
		LensId: func() *string {
			value := "not-a-uuid"
			return &value
		}(),
	})

	err := handler.StreamKnowledgeHomeUpdates(testUserContext(), req, nil)
	require.Error(t, err)

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestHandler_InitialStreamSeq_UsesLatestKnownSeq(t *testing.T) {
	logger.InitLogger()
	handler := &Handler{
		latestSeqPort: &mockLatestSeqPort{seq: 42},
		logger:        slog.Default(),
	}

	seq, err := handler.initialStreamSeq(context.Background(), uuid.New(), uuid.New())

	require.NoError(t, err)
	assert.Equal(t, int64(42), seq)
}

func TestHighWaterMark_ExceedsThreshold_SendsDigestChanged(t *testing.T) {
	// When coalesced events exceed streamHighWaterMark (10), the handler
	// should send a single digest_changed instead of individual events.

	// Generate 15 unique aggregate events (all survive coalescing)
	events := make([]domain.KnowledgeEvent, 15)
	for i := range events {
		events[i] = domain.KnowledgeEvent{
			EventType:     domain.EventArticleCreated,
			AggregateType: "article",
			AggregateID:   uuid.New().String(),
			EventSeq:      int64(i + 1),
			OccurredAt:    time.Now(),
		}
	}

	coalesced := coalesceStreamEvents(events)
	assert.Len(t, coalesced, 15, "15 unique aggregates should all survive coalescing")
	assert.Greater(t, len(coalesced), streamHighWaterMark,
		"coalesced count should exceed streamHighWaterMark threshold")
}

func TestHighWaterMark_BelowThreshold_IndividualEvents(t *testing.T) {
	// When coalesced events are within streamHighWaterMark, individual events are sent.
	events := make([]domain.KnowledgeEvent, 5)
	for i := range events {
		events[i] = domain.KnowledgeEvent{
			EventType:     domain.EventArticleCreated,
			AggregateType: "article",
			AggregateID:   uuid.New().String(),
			EventSeq:      int64(i + 1),
			OccurredAt:    time.Now(),
		}
	}

	coalesced := coalesceStreamEvents(events)
	assert.Len(t, coalesced, 5)
	assert.LessOrEqual(t, len(coalesced), streamHighWaterMark,
		"coalesced count should be within streamHighWaterMark threshold")

	for _, e := range coalesced {
		resp := buildStreamResponse(e)
		assert.Equal(t, "item_added", resp.EventType)
		assert.NotNil(t, resp.Item)
	}
}

func TestHighWaterMark_CoalescingReducesBelowThreshold(t *testing.T) {
	// 20 events for the same aggregate coalesce to 1 → below threshold
	events := make([]domain.KnowledgeEvent, 20)
	aggID := uuid.New().String()
	for i := range events {
		events[i] = domain.KnowledgeEvent{
			EventType:     domain.EventSummaryVersionCreated,
			AggregateType: "article",
			AggregateID:   aggID,
			EventSeq:      int64(i + 1),
			OccurredAt:    time.Now(),
		}
	}

	coalesced := coalesceStreamEvents(events)
	assert.Len(t, coalesced, 1, "same aggregate deduplicates to 1")
	assert.LessOrEqual(t, len(coalesced), streamHighWaterMark,
		"after coalescing, should be within threshold → send individual events")
}

func TestEnrichRecallChangedUpdate_ReplacesMinimalPayloadWhenCandidateResolved(t *testing.T) {
	userID := uuid.New()
	articleID := uuid.New()
	publishedAt := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	handler := &Handler{
		recallRailUsecase: recall_rail_usecase.NewRecallRailUsecase(&mockRecallCandidatesPort{
			candidates: []domain.RecallCandidate{
				{
					ItemKey:     "article:" + articleID.String(),
					RecallScore: 0.91,
					Reasons: []domain.RecallReason{
						{Type: domain.ReasonOpenedNotRevisited, Description: "Opened before"},
					},
					Item: &domain.KnowledgeHomeItem{
						ItemKey:      "article:" + articleID.String(),
						ItemType:     domain.ItemArticle,
						Title:        "Recovered title",
						SummaryState: domain.SummaryStateMissing,
						PublishedAt:  &publishedAt,
					},
				},
			},
		}, nil),
		logger: slog.Default(),
	}

	update := buildStreamResponse(domain.KnowledgeEvent{
		EventType:     domain.EventRecallSnoozed,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID.String(),
		OccurredAt:    time.Now(),
	})

	handler.enrichRecallChangedUpdate(context.Background(), userID, update)

	require.NotNil(t, update.RecallChange)
	assert.Equal(t, "article:"+articleID.String(), update.RecallChange.ItemKey)
	assert.Equal(t, 0.91, update.RecallChange.RecallScore)
	require.NotNil(t, update.RecallChange.Item)
	assert.Equal(t, "Recovered title", update.RecallChange.Item.Title)
}

// testAuthInterceptor injects a user context for httptest-based streaming tests.
type testAuthInterceptor struct{}

func (i *testAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		user := &domain.UserContext{
			UserID:    uuid.MustParse("93852825-3755-4c9b-af19-ac92002ebf82"),
			Email:     "test@example.com",
			Role:      domain.UserRoleUser,
			TenantID:  uuid.New(),
			SessionID: "test-session",
			LoginAt:   time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		return next(domain.SetUserContext(ctx, user), req)
	}
}

func (i *testAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *testAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		user := &domain.UserContext{
			UserID:    uuid.MustParse("93852825-3755-4c9b-af19-ac92002ebf82"),
			Email:     "test@example.com",
			Role:      domain.UserRoleUser,
			TenantID:  uuid.New(),
			SessionID: "test-session",
			LoginAt:   time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		return next(domain.SetUserContext(ctx, user), conn)
	}
}

func TestStreamKnowledgeHomeUpdates_SendsImmediateHeartbeat(t *testing.T) {
	logger.InitLogger()

	flagPort := &mockFeatureFlagPort{
		enabledFlags: map[string]bool{
			domain.FlagStreamUpdates: true,
		},
	}
	eventsForUser := &mockEventsForUserPort{events: nil, err: nil}
	handler := NewHandler(
		nil, nil, nil, // home, seen, action
		nil, nil, nil, // recall: rail, snooze, dismiss
		nil, nil, nil, nil, nil, // lens
		nil,           // eventsPort
		eventsForUser, // eventsForUserPort
		nil,           // lensVisibilityPort
		nil,           // resolveLensPort
		flagPort,      // featureFlagPort
		nil,           // metrics
		slog.Default(),
	)

	path, h := knowledgehomev1connect.NewKnowledgeHomeServiceHandler(
		handler,
		connect.WithInterceptors(&testAuthInterceptor{}),
	)
	mux := http.NewServeMux()
	mux.Handle(path, h)

	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	client := knowledgehomev1connect.NewKnowledgeHomeServiceClient(
		server.Client(),
		server.URL,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.StreamKnowledgeHomeUpdates(
		ctx,
		connect.NewRequest(&knowledgehomev1.StreamKnowledgeHomeUpdatesRequest{}),
	)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	// First message must be an immediate heartbeat (not delayed by 5-10s tickers)
	started := time.Now()
	require.True(t, stream.Receive(), "expected first message from stream")
	elapsed := time.Since(started)

	msg := stream.Msg()
	assert.Equal(t, "heartbeat", msg.EventType, "first message should be a heartbeat")
	assert.NotEmpty(t, msg.OccurredAt, "heartbeat should have occurred_at timestamp")
	assert.Less(t, elapsed, 2*time.Second, "first message should arrive within 2s, not after 5-10s ticker delay")
}
