package augur_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"rag-orchestrator/internal/adapter/connect/augur"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// failingEmitter implements usecase.KnowledgeEventEmitter and always fails the
// EmitAugurConversationLinked call. It exercises the warn-and-continue path:
// the handler must keep returning success to the caller (best-effort emit) but
// must also bump the failure counter so the rollout is observable.
type failingEmitter struct{}

func (failingEmitter) EmitAugurConversationLinked(_ context.Context, _ usecase.AugurConversationLinkedInput) error {
	return errors.New("simulated AppendKnowledgeEvent error")
}

// TestCreateAugurSessionFromLoopEntry_EmitFailureBumpsFailureCounter pins the
// emitter-failure observability contract for Knowledge Loop Completion Phase 1.
//
//   - The conversation creation path MUST keep returning success to the caller
//     (best-effort emit, see ADR-000855).
//   - rag_orchestrator_knowledge_event_emitter_failure_total{event_type=
//     "augur.conversation_linked.v1"} MUST increment by exactly 1 per failed
//     emit, distinct from the projector's event_dropped_total.
//
// Without the counter, an emit degradation in production would only surface as
// a warn log line, which Prometheus alerts cannot page on at low rates.
func TestCreateAugurSessionFromLoopEntry_EmitFailureBumpsFailureCounter(t *testing.T) {
	mockConv := new(MockAugurConversationUsecase)
	userID := uuid.New()
	createdID := uuid.New()

	mockConv.On("CreateSessionFromLoopEntry", mock.Anything, mock.Anything).
		Return(&domain.AugurConversation{ID: createdID, UserID: userID}, nil)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := augur.NewHandler(
		new(MockAnswerWithRAGUsecase),
		new(MockRetrieveContextUsecase),
		mockConv,
		failingEmitter{},
		logger,
	)

	req := newLoopRequest(userID, &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		LensModeId:        "default",
		WhyText:           "A fresh why.",
	})

	// Pre-read counter — collectAndScrape is the only way to read a
	// process-wide CounterVec without exporting the underlying metric.
	const metricName = "rag_orchestrator_knowledge_event_emitter_failure_total"
	const eventType = "augur.conversation_linked.v1"
	before := scrapeCounter(t, metricName, eventType)

	resp, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.NoError(t, err, "emit failure must NOT fail conversation creation (warn-and-continue)")
	require.NotNil(t, resp)
	require.Equal(t, createdID.String(), resp.Msg.ConversationId)

	after := scrapeCounter(t, metricName, eventType)
	if after-before != 1 {
		t.Errorf("%s{event_type=%q} delta = %v; want 1", metricName, eventType, after-before)
	}
}

// scrapeCounter reads the current value of a CounterVec from the default
// Prometheus registry. We can't use testutil.ToFloat64 directly because the
// counter is package-private to sovereign_client; gathering the registry
// gives us a read path that doesn't require exporting the metric handle.
func scrapeCounter(t *testing.T, name, eventType string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if hasLabel(m.GetLabel(), "event_type", eventType) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func hasLabel(labels []*dto.LabelPair, key, value string) bool {
	for _, l := range labels {
		if l.GetName() == key && l.GetValue() == value {
			return true
		}
	}
	return false
}
