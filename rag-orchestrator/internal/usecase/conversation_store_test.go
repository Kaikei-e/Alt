package usecase

import (
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestConversationStore_GetReturnsNilForUnknownThread(t *testing.T) {
	store := NewConversationStore(100, 30*time.Minute)

	got := store.Get("unknown-thread-id")
	assert.Nil(t, got)
}

func TestConversationStore_PutAndGet(t *testing.T) {
	store := NewConversationStore(100, 30*time.Minute)

	state := &domain.ConversationState{
		ThreadID:         "thread-1",
		Mode:             domain.ModeArticleScoped,
		CurrentArticleID: "article-123",
		TurnCount:        1,
	}
	store.Put(state)

	got := store.Get("thread-1")
	assert.NotNil(t, got)
	assert.Equal(t, "thread-1", got.ThreadID)
	assert.Equal(t, domain.ModeArticleScoped, got.Mode)
	assert.Equal(t, "article-123", got.CurrentArticleID)
	assert.Equal(t, 1, got.TurnCount)
}

func TestConversationStore_PutOverwritesExisting(t *testing.T) {
	store := NewConversationStore(100, 30*time.Minute)

	store.Put(&domain.ConversationState{
		ThreadID:  "thread-1",
		TurnCount: 1,
	})
	store.Put(&domain.ConversationState{
		ThreadID:  "thread-1",
		TurnCount: 2,
	})

	got := store.Get("thread-1")
	assert.NotNil(t, got)
	assert.Equal(t, 2, got.TurnCount)
}

func TestConversationStore_Reset(t *testing.T) {
	store := NewConversationStore(100, 30*time.Minute)

	store.Put(&domain.ConversationState{
		ThreadID:  "thread-1",
		TurnCount: 5,
	})
	store.Reset("thread-1")

	got := store.Get("thread-1")
	assert.Nil(t, got)
}

func TestConversationStore_TTLExpiry(t *testing.T) {
	store := NewConversationStore(100, 50*time.Millisecond)

	store.Put(&domain.ConversationState{
		ThreadID:  "thread-1",
		TurnCount: 1,
	})

	// Should be available immediately
	assert.NotNil(t, store.Get("thread-1"))

	// Wait for TTL expiry
	time.Sleep(100 * time.Millisecond)

	got := store.Get("thread-1")
	assert.Nil(t, got, "state should expire after TTL")
}

func TestConversationStore_LRUEviction(t *testing.T) {
	store := NewConversationStore(2, 30*time.Minute)

	store.Put(&domain.ConversationState{ThreadID: "thread-1"})
	store.Put(&domain.ConversationState{ThreadID: "thread-2"})
	store.Put(&domain.ConversationState{ThreadID: "thread-3"})

	// thread-1 should be evicted (LRU)
	assert.Nil(t, store.Get("thread-1"))
	assert.NotNil(t, store.Get("thread-2"))
	assert.NotNil(t, store.Get("thread-3"))
}
