package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
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

func (s *stubKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) error {
	s.events = append(s.events, event)
	return s.err
}

func TestEmitArticleCreatedEvent(t *testing.T) {
	logger.InitLogger()
	t.Run("emits ArticleCreated for valid payload", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{}
		articleID := uuid.New().String()
		userID := uuid.New().String()
		payload, _ := json.Marshal(map[string]interface{}{
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

	t.Run("skips when port is nil", func(t *testing.T) {
		// Should not panic
		emitArticleCreatedEvent(context.Background(), nil, []byte(`{"article_id":"x"}`))
	})

	t.Run("skips on invalid user_id", func(t *testing.T) {
		stub := &stubKnowledgeEventPort{}
		payload, _ := json.Marshal(map[string]interface{}{
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
		payload, _ := json.Marshal(map[string]interface{}{
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
