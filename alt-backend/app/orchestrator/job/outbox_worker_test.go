package job

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		emitArticleCreatedEvent(context.Background(), stub, payload)

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
			emitArticleCreatedEvent(context.Background(), nil, []byte(`{"article_id":"x"}`))
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

		emitArticleCreatedEvent(context.Background(), stub, payload)

		assert.Empty(t, stub.events)
	})

	t.Run("continues on append error", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{err: assert.AnError}
		payload, _ := json.Marshal(map[string]any{
			"article_id": uuid.New().String(),
			"url":        "http://example.com",
			"title":      "Test",
			"user_id":    uuid.New().String(),
		})

		// Should not panic
		emitArticleCreatedEvent(context.Background(), stub, payload)
		assert.Len(t, stub.events, 1) // event was attempted
	})
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

func TestProcessOutboxEvents_CancelMidProcessing_MarksInFlightEventFailed(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &blockingRagIntegration{started: make(chan struct{})}
	knowledgePort := &stubKnowledgeEventPort{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processOutboxEvents(ctx, repo, rag, knowledgePort)
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
		done <- processOutboxEvents(ctx, repo, rag, knowledgePort)
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
