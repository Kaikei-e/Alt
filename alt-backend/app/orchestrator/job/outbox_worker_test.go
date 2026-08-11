package job

import (
	"alt/adapter/augur_adapter"
	"alt/domain"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// fakeRagClient implements augur_adapter.RagClientInterface by embedding it
// as a nil interface value, so any endpoint this test doesn't override
// panics loudly if AugurAdapter ever calls it. Only UpsertIndexWithResponse
// is overridden — the sole endpoint the outbox worker's real call path uses
// (see augur_adapter.go's doc comment on UpsertArticle).
type fakeRagClient struct {
	augur_adapter.RagClientInterface
	upsertIndex func(ctx context.Context, body rag_gateway.UpsertIndexJSONRequestBody, reqEditors ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error)
}

func (f *fakeRagClient) UpsertIndexWithResponse(ctx context.Context, body rag_gateway.UpsertIndexJSONRequestBody, reqEditors ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error) {
	return f.upsertIndex(ctx, body, reqEditors...)
}

type stubKnowledgeEventPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (s *stubKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	s.events = append(s.events, event)
	if s.err != nil {
		return 0, s.err
	}
	return int64(len(s.events)), nil
}

func TestEmitArticleCreatedEvent(t *testing.T) {
	logger.InitLogger()
	t.Run("emits ArticleCreated for valid payload", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{}
		articleID := uuid.New().String()
		userID := uuid.New().String()
		payload, _ := json.Marshal(map[string]any{
			"article_id": articleID,
			"url":        "http://example.com/article",
			"title":      "Test Article",
			"user_id":    userID,
			"updated_at": time.Now().Format(time.RFC3339),
		})

		require.NoError(t, emitArticleCreatedEvent(context.Background(), stub, payload))

		require.Len(t, stub.events, 1)
		ev := stub.events[0]
		assert.Equal(t, domain.EventArticleCreated, ev.EventType)
		assert.Equal(t, articleID, ev.AggregateID)
		assert.Equal(t, "article-created:"+articleID, ev.DedupeKey)
		assert.Equal(t, domain.ActorService, ev.ActorType)
		assert.Equal(t, "outbox-worker", ev.ActorID)
	})

	t.Run("panics when port is nil (wiring bug must be loud, not silent)", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = emitArticleCreatedEvent(context.Background(), nil, []byte(`{"article_id":"x"}`))
		})
	})

	t.Run("skips on invalid user_id", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{}
		payload, _ := json.Marshal(map[string]any{
			"article_id": uuid.New().String(),
			"url":        "http://example.com",
			"title":      "Test",
			"user_id":    "not-a-uuid",
		})

		require.NoError(t, emitArticleCreatedEvent(context.Background(), stub, payload))

		assert.Empty(t, stub.events)
	})

	t.Run("returns append error so the caller can withhold PROCESSED", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{err: assert.AnError}
		payload, _ := json.Marshal(map[string]any{
			"article_id": uuid.New().String(),
			"url":        "http://example.com",
			"title":      "Test",
			"user_id":    uuid.New().String(),
		})

		err := emitArticleCreatedEvent(context.Background(), stub, payload)
		require.Error(t, err, "append failure must surface so processOutboxEvents can withhold PROCESSED")
		assert.Len(t, stub.events, 1) // event was attempted
	})
}

// successRagIntegration is a RagIntegrationPort that always succeeds UpsertArticle.
type successRagIntegration struct{}

func (s *successRagIntegration) RetrieveContext(_ context.Context, _ string, _ []string) ([]rag_integration_port.RagContext, error) {
	return nil, nil
}

func (s *successRagIntegration) UpsertArticle(_ context.Context, _ rag_integration_port.UpsertArticleInput) error {
	return nil
}

func (s *successRagIntegration) Answer(_ context.Context, _ rag_integration_port.AnswerInput) (<-chan string, error) {
	return nil, nil
}

// TestProcessOutboxEvents_RagOK_SovereignAppendFails_DoesNotMarkProcessed pins
// the ACK-before-all-side-effects defect: RAG success used to mark the shared
// ARTICLE_UPSERT row PROCESSED before emitArticleCreatedEvent ran, and append
// failures were WARN/non-fatal. The observed Trail article:<uuid> titles came
// from that silent loss (PROCESSED outbox + no ArticleCreated → blank Home
// title/url → Trail SQL falls back to item_key).
func TestProcessOutboxEvents_RagOK_SovereignAppendFails_DoesNotMarkProcessed(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &successRagIntegration{}
	knowledgePort := &stubKnowledgeEventPort{err: fmt.Errorf("sovereign AppendKnowledgeEvent: unavailable")}

	err := processOutboxEvents(context.Background(), repo, rag, knowledgePort, newOutboxRetryTracker())
	require.NoError(t, err)

	assert.Equal(t, "PENDING", repo.statusOf(eventID),
		"RAG OK + sovereign append failure must release for retry, not leave terminal PROCESSED without ArticleCreated")
	require.Len(t, knowledgePort.events, 1, "ArticleCreated append must still be attempted")
}

// TestProcessOutboxEvents_RagOK_SovereignAppendOK_MarksProcessed is the
// complementary happy path: both side effects durable → PROCESSED.
func TestProcessOutboxEvents_RagOK_SovereignAppendOK_MarksProcessed(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &successRagIntegration{}
	knowledgePort := &stubKnowledgeEventPort{}

	err := processOutboxEvents(context.Background(), repo, rag, knowledgePort, newOutboxRetryTracker())
	require.NoError(t, err)

	assert.Equal(t, "PROCESSED", repo.statusOf(eventID))
	require.Len(t, knowledgePort.events, 1)
	assert.Equal(t, domain.EventArticleCreated, knowledgePort.events[0].EventType)
}

// TestProcessOutboxEvents_SovereignAppendFailsThenSucceeds_RetriesAndAcks
// proves the release path is reclaimable: a failed append leaves the row
// retryable, and a later tick with a healthy sovereign marks PROCESSED.
func TestProcessOutboxEvents_SovereignAppendFailsThenSucceeds_RetriesAndAcks(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &successRagIntegration{}
	knowledgePort := &stubKnowledgeEventPort{err: fmt.Errorf("sovereign AppendKnowledgeEvent: unavailable")}
	retries := newOutboxRetryTracker()

	require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
	assert.Equal(t, "PENDING", repo.statusOf(eventID))
	require.Len(t, knowledgePort.events, 1)

	knowledgePort.err = nil
	require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
	assert.Equal(t, "PROCESSED", repo.statusOf(eventID))
	require.Len(t, knowledgePort.events, 2, "retry must attempt ArticleCreated again (dedupe-safe upstream)")
}

// mockOutboxRepo is an in-memory stand-in for datahub_gateway.OutboxGateway,
// which reaches alt-data-hub over Connect-RPC in production (ADR-000954
// Wave 3). It records what the worker asked for, not how the provider stored
// it: Release is a distinct method from MarkProcessed, so the assertions below
// distinguish "gave up on this event" from "handed it back untouched" — which
// a single status-string setter could not.
type mockOutboxRepo struct {
	mu            sync.Mutex
	events        []domain.OutboxEvent
	statusUpdates []outboxStatusUpdate
}

type outboxStatusUpdate struct {
	id     string
	status string
}

func (m *mockOutboxRepo) ClaimBatch(_ context.Context, _ int) ([]domain.OutboxEvent, error) {
	return m.events, nil
}

func (m *mockOutboxRepo) MarkProcessed(_ context.Context, id string, status domain.OutboxEventStatus, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates = append(m.statusUpdates, outboxStatusUpdate{id: id, status: string(status)})
	return nil
}

func (m *mockOutboxRepo) Release(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates = append(m.statusUpdates, outboxStatusUpdate{id: id, status: string(domain.OutboxPending)})
	return nil
}

// statusOf returns the most recently written status for id, or "" if never written.
func (m *mockOutboxRepo) statusOf(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := ""
	for _, u := range m.statusUpdates {
		if u.id == id {
			status = u.status
		}
	}
	return status
}

// blockingRagIntegration simulates the local embedder taking longer than the
// job timeout: UpsertArticle only returns once the caller's context is done,
// the same way a 500KB+ article with 100+ chunks legitimately can.
type blockingRagIntegration struct {
	started chan struct{}
}

func (b *blockingRagIntegration) RetrieveContext(_ context.Context, _ string, _ []string) ([]rag_integration_port.RagContext, error) {
	return nil, nil
}

func (b *blockingRagIntegration) UpsertArticle(ctx context.Context, _ rag_integration_port.UpsertArticleInput) error {
	close(b.started)
	<-ctx.Done()
	return ctx.Err()
}

func (b *blockingRagIntegration) Answer(_ context.Context, _ rag_integration_port.AnswerInput) (<-chan string, error) {
	return nil, nil
}

// alwaysFailRagIntegration returns a fixed error from every UpsertArticle
// call, standing in for augur_adapter having already classified a real
// rag-orchestrator response (see augur_adapter_test.go for that
// classification itself).
type alwaysFailRagIntegration struct {
	err error
}

func (a *alwaysFailRagIntegration) RetrieveContext(_ context.Context, _ string, _ []string) ([]rag_integration_port.RagContext, error) {
	return nil, nil
}

func (a *alwaysFailRagIntegration) UpsertArticle(_ context.Context, _ rag_integration_port.UpsertArticleInput) error {
	return a.err
}

func (a *alwaysFailRagIntegration) Answer(_ context.Context, _ rag_integration_port.AnswerInput) (<-chan string, error) {
	return nil, nil
}

// outboxUpsertEventFixture mirrors the raw map payload save_article_driver.go
// actually enqueues (snake_case keys), not the Go-native UpsertArticleInput
// struct, so it also exercises emitArticleCreatedEvent's payload parsing.
func outboxUpsertEventFixture(id string) domain.OutboxEvent {
	payload, _ := json.Marshal(map[string]any{
		"article_id": id,
		"url":        "https://example.com/" + id,
		"title":      "Test Article",
		"body":       "heavy article body",
		"user_id":    uuid.New().String(),
		"updated_at": time.Now().Format(time.RFC3339),
	})
	return domain.OutboxEvent{
		ID:        id,
		EventType: "ARTICLE_UPSERT",
		Payload:   payload,
		Status:    domain.OutboxProcessing,
	}
}

// TestProcessOutboxEvents_CancelMidProcessing_MarksInFlightEventFailed is a
// worker-layer unit test: GIVEN a plain (non-transient) error from
// UpsertArticle — which is what a job-timeout cancellation now produces
// through the real classifier, see augur_adapter_test.go's "context deadline
// exceeded" case and the integration test right below this one — the worker
// must mark the row FAILED, not leave it PROCESSING. blockingRagIntegration
// stands in for any RagIntegrationPort implementation raising that kind of
// error; it does not by itself prove augur_adapter classifies a caller-side
// timeout this way, which is why the adapter-level and integration tests
// exist as a separate, non-bypassable check on that classification.
func TestProcessOutboxEvents_CancelMidProcessing_MarksInFlightEventFailed(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &blockingRagIntegration{started: make(chan struct{})}
	knowledgePort := &stubKnowledgeEventPort{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processOutboxEvents(ctx, repo, rag, knowledgePort, newOutboxRetryTracker())
	}()

	select {
	case <-rag.started:
	case <-time.After(5 * time.Second):
		t.Fatal("UpsertArticle was never called")
	}
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("processOutboxEvents did not return promptly after context cancellation")
	}

	assert.Equal(t, "FAILED", repo.statusOf(eventID), "in-flight event must end FAILED, not stuck PROCESSING")
}

// TestProcessOutboxEvents_JobTimeoutMidUpsert_ThroughRealAdapter_EndsFailedNotRetried
// is the integration check the test above cannot be: it wires the real
// augur_adapter.AugurAdapter (not a stub that bypasses its classification) so
// a mutation reinstating defect 2 — augur_adapter marking a caller-context
// cancellation transient — actually turns this test red. Without going
// through the real adapter, a job-timeout mid-upsert would be classified
// ErrRagUpsertTransient and released to PENDING instead of failing once,
// costing the worker another full job timeout on the very next tick (see
// augur_adapter.go's transport-error branch).
func TestProcessOutboxEvents_JobTimeoutMidUpsert_ThroughRealAdapter_EndsFailedNotRetried(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	knowledgePort := &stubKnowledgeEventPort{}

	started := make(chan struct{})
	client := &fakeRagClient{
		upsertIndex: func(ctx context.Context, _ rag_gateway.UpsertIndexJSONRequestBody, _ ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error) {
			close(started)
			<-ctx.Done() // the job timeout fires while the HTTP call is in flight
			return nil, ctx.Err()
		},
	}
	rag := augur_adapter.NewAugurAdapter(client)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processOutboxEvents(ctx, repo, rag, knowledgePort, newOutboxRetryTracker())
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("UpsertArticle was never called")
	}
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("processOutboxEvents did not return promptly after context cancellation")
	}

	assert.Equal(t, "FAILED", repo.statusOf(eventID),
		"a job-timeout mid-upsert, classified by the real adapter, must end FAILED on the first attempt — not be released for a same-worker retry that burns another full job timeout")
}

func TestProcessOutboxEvents_CancelMidBatch_ResetsUnattemptedClaimedEventsToPending(t *testing.T) {
	logger.InitLogger()

	blockedID := uuid.New().String()
	unattemptedID1 := uuid.New().String()
	unattemptedID2 := uuid.New().String()

	repo := &mockOutboxRepo{events: []domain.OutboxEvent{
		outboxUpsertEventFixture(blockedID),
		outboxUpsertEventFixture(unattemptedID1),
		outboxUpsertEventFixture(unattemptedID2),
	}}
	rag := &blockingRagIntegration{started: make(chan struct{})}
	knowledgePort := &stubKnowledgeEventPort{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processOutboxEvents(ctx, repo, rag, knowledgePort, newOutboxRetryTracker())
	}()

	select {
	case <-rag.started:
	case <-time.After(5 * time.Second):
		t.Fatal("UpsertArticle was never called")
	}
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("processOutboxEvents did not return promptly after context cancellation")
	}

	assert.Equal(t, "FAILED", repo.statusOf(blockedID), "in-flight event must end FAILED, not stuck PROCESSING")
	assert.Equal(t, "PENDING", repo.statusOf(unattemptedID1), "unattempted claimed event must be released back to PENDING")
	assert.Equal(t, "PENDING", repo.statusOf(unattemptedID2), "unattempted claimed event must be released back to PENDING")
}

// TestProcessOutboxEvents_TransientRagFailure_ReleasesForRetry pins the fix
// for the finding this file exists to regression-test: a transient RAG
// failure (5xx / transport, classified by augur_adapter and signaled via
// rag_integration_port.ErrRagUpsertTransient) used to take the same terminal
// branch as a permanent one, so a downstream outage permanently dropped
// every article it touched. It must instead go back to PENDING so the next
// tick tries again.
func TestProcessOutboxEvents_TransientRagFailure_ReleasesForRetry(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &alwaysFailRagIntegration{err: fmt.Errorf("%w: RAG UpsertIndex returned status 500", rag_integration_port.ErrRagUpsertTransient)}
	knowledgePort := &stubKnowledgeEventPort{}

	err := processOutboxEvents(context.Background(), repo, rag, knowledgePort, newOutboxRetryTracker())

	require.NoError(t, err)
	assert.Equal(t, "PENDING", repo.statusOf(eventID),
		"a transient RAG failure must be released for retry, not marked terminally FAILED")
}

// TestProcessOutboxEvents_NonTransientRagFailure_StaysFailed is the other
// half of the taxonomy: an error augur_adapter did not mark transient (a 4xx,
// or any RagIntegrationPort implementation that returns a plain error) keeps
// the pre-fix terminal behavior, because retrying it cannot succeed.
func TestProcessOutboxEvents_NonTransientRagFailure_StaysFailed(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &alwaysFailRagIntegration{err: fmt.Errorf("RAG UpsertIndex returned non-OK status: 400")}
	knowledgePort := &stubKnowledgeEventPort{}

	err := processOutboxEvents(context.Background(), repo, rag, knowledgePort, newOutboxRetryTracker())

	require.NoError(t, err)
	assert.Equal(t, "FAILED", repo.statusOf(eventID), "a non-transient RAG failure must stay terminally FAILED")
}

// TestProcessOutboxEvents_TransientRagFailure_ExhaustsRetriesThenFails proves
// the retry path is bounded: a downstream outage that outlives
// maxOutboxUpsertAttempts still ends the row at FAILED instead of retrying
// forever and starving newer PENDING rows behind it in claim order (the
// worker claims oldest-first).
func TestProcessOutboxEvents_TransientRagFailure_ExhaustsRetriesThenFails(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &alwaysFailRagIntegration{err: fmt.Errorf("%w: RAG UpsertIndex returned status 503", rag_integration_port.ErrRagUpsertTransient)}
	knowledgePort := &stubKnowledgeEventPort{}
	retries := newOutboxRetryTracker()

	for attempt := 1; attempt <= maxOutboxUpsertAttempts; attempt++ {
		require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
		if attempt < maxOutboxUpsertAttempts {
			require.Equal(t, "PENDING", repo.statusOf(eventID), "attempt %d of %d must still be retried", attempt, maxOutboxUpsertAttempts)
		}
	}

	assert.Equal(t, "FAILED", repo.statusOf(eventID),
		"a persistently failing transient error must end FAILED once attempts are exhausted, not retry forever")
}

// TestProcessOutboxEvents_TransientRagFailure_SurvivesRealisticRedeployWindow
// pins the fix for defect 1: the original 3-attempt budget against a 5s tick
// interval covered only ~10s, so any real redeploy (observed 20-60s) still
// reproduced the incident this fix exists to prevent. This simulates one
// tick per outboxWorkerTickInterval of continuous transient failure across a
// 60s window and asserts the row is still retryable at the end, not
// terminally FAILED.
func TestProcessOutboxEvents_TransientRagFailure_SurvivesRealisticRedeployWindow(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &alwaysFailRagIntegration{err: fmt.Errorf("%w: RAG UpsertIndex returned status 503", rag_integration_port.ErrRagUpsertTransient)}
	knowledgePort := &stubKnowledgeEventPort{}
	retries := newOutboxRetryTracker()

	const observedWorstCaseRedeploy = 60 * time.Second
	ticksInWindow := int(observedWorstCaseRedeploy / outboxWorkerTickInterval)

	for tick := 0; tick < ticksInWindow; tick++ {
		require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
	}

	assert.Equal(t, "PENDING", repo.statusOf(eventID),
		"a %s outage (%d ticks) must not exhaust the %d-attempt retry budget", observedWorstCaseRedeploy, ticksInWindow, maxOutboxUpsertAttempts)
}

// releaseFailingOutboxRepo simulates the release-to-PENDING RPC itself
// failing (e.g. alt-data-hub unreachable at the exact moment of the release
// call) — as opposed to the UpsertArticle RPC failing, which is what the
// other fixtures in this file simulate.
type releaseFailingOutboxRepo struct {
	*mockOutboxRepo
}

func (r *releaseFailingOutboxRepo) Release(_ context.Context, _ string) error {
	return fmt.Errorf("release rpc: connection refused")
}

// sumCounterValue collects every int64 Sum data point recorded under name,
// regardless of attributes, across an already-Collect()ed ResourceMetrics.
func sumCounterValue(rm metricdata.ResourceMetrics, name string) int64 {
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

// TestHandleUpsertFailure_ReleaseRPCFailure_NotCountedAsRetried pins the fix
// for defect 3: outboxRetriedCounter must not report a retry for a row whose
// Release RPC itself failed — that row is a PROCESSING zombie invisible to
// both the PENDING claim query and a FAILED-status audit, not a row the next
// tick will actually reprocess. It reads the real OTel counters through a
// ManualReader wired into the package's own meter (not a proxy assertion),
// so a mutation that increments outboxRetriedCounter unconditionally again
// turns this test red on the counter value itself.
func TestHandleUpsertFailure_ReleaseRPCFailure_NotCountedAsRetried(t *testing.T) {
	logger.InitLogger()

	reader := sdkmetric.NewManualReader()
	prevProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	outboxMeterOnce = sync.Once{}
	t.Cleanup(func() {
		otel.SetMeterProvider(prevProvider)
		outboxMeterOnce = sync.Once{}
	})

	eventID := uuid.New().String()
	repo := &releaseFailingOutboxRepo{mockOutboxRepo: &mockOutboxRepo{
		events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)},
	}}
	rag := &alwaysFailRagIntegration{err: fmt.Errorf("%w: RAG UpsertIndex returned status 503", rag_integration_port.ErrRagUpsertTransient)}
	knowledgePort := &stubKnowledgeEventPort{}

	err := processOutboxEvents(context.Background(), repo, rag, knowledgePort, newOutboxRetryTracker())
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	assert.Equal(t, int64(0), sumCounterValue(rm, "alt_harvester_outbox_events_retried_total"),
		"a failed Release RPC must not be counted as a successful retry")
	assert.Equal(t, int64(1), sumCounterValue(rm, "alt_harvester_outbox_events_release_failed_total"),
		"a failed Release RPC must surface on its own counter so the PROCESSING zombie it leaves behind is observable")
}

// TestProcessOutboxEvents_EnqueuedPayloadReachesUpsertIntact pins the wire
// contract between the payload save_article_driver enqueues (snake_case map
// keys) and the body augur_adapter sends to rag-orchestrator, by driving the
// real adapter with the raw bytes rather than a Go-native input struct.
//
// encoding/json's case-insensitive fallback does not cross underscores, so a
// field whose JSON key differs from its Go name by more than case arrives
// empty instead of erroring. rag-orchestrator answers a blank article_id with
// 400, which the outbox classifies terminal — every article stops reaching
// the index, and Augur retrieves nothing. Every other test in this file
// either builds UpsertArticleInput in Go or discards it, so this is the only
// place that boundary is crossed.
func TestProcessOutboxEvents_EnqueuedPayloadReachesUpsertIntact(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	articleID := uuid.New().String()
	userID := uuid.New().String()
	updatedAt := time.Now().UTC().Truncate(time.Second)

	payload, err := json.Marshal(map[string]any{
		"article_id": articleID,
		"url":        "https://example.com/article",
		"title":      "Test Article",
		"body":       "article body",
		"user_id":    userID,
		"updated_at": updatedAt.Format(time.RFC3339),
	})
	require.NoError(t, err)

	repo := &mockOutboxRepo{events: []domain.OutboxEvent{{
		ID:        eventID,
		EventType: "ARTICLE_UPSERT",
		Payload:   payload,
		Status:    domain.OutboxProcessing,
	}}}

	var sent rag_gateway.UpsertIndexJSONRequestBody
	client := &fakeRagClient{
		upsertIndex: func(_ context.Context, body rag_gateway.UpsertIndexJSONRequestBody, _ ...rag_gateway.RequestEditorFn) (*rag_gateway.UpsertIndexResponse, error) {
			sent = body
			return &rag_gateway.UpsertIndexResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}, nil
		},
	}

	require.NoError(t, processOutboxEvents(context.Background(), repo,
		augur_adapter.NewAugurAdapter(client), &stubKnowledgeEventPort{}, newOutboxRetryTracker()))

	assert.Equal(t, articleID, sent.ArticleId,
		"article_id must survive the enqueued-payload round-trip: rag-orchestrator rejects a blank one with 400, and the outbox treats that 4xx as terminal")
	assert.Equal(t, userID, sent.UserId,
		"user_id must survive: it is the only tenant scoping the RAG index has")
	require.NotNil(t, sent.UpdatedAt, "updated_at must survive: it is the article-upsert fact's own timestamp")
	assert.Equal(t, updatedAt, sent.UpdatedAt.UTC())
	assert.Equal(t, "https://example.com/article", sent.Url)
	assert.Equal(t, "Test Article", sent.Title)
	assert.Equal(t, "article body", sent.Body)
	assert.Equal(t, "PROCESSED", repo.statusOf(eventID))
}
