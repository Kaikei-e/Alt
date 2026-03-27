package usecase

import (
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// ConversationStore manages conversation state per thread using an in-memory LRU cache.
type ConversationStore struct {
	states *expirable.LRU[string, *domain.ConversationState]
}

// NewConversationStore creates a store with bounded capacity and TTL-based expiry.
func NewConversationStore(maxSize int, ttl time.Duration) *ConversationStore {
	panic("not implemented")
}

// Get returns the conversation state for a thread, or nil if not found.
func (s *ConversationStore) Get(threadID string) *domain.ConversationState {
	panic("not implemented")
}

// Put stores or updates the conversation state for a thread.
func (s *ConversationStore) Put(state *domain.ConversationState) {
	panic("not implemented")
}

// Reset removes the conversation state for a thread, starting fresh.
func (s *ConversationStore) Reset(threadID string) {
	panic("not implemented")
}
