package repository

import (
	"testing"

	"rag-orchestrator/internal/domain"
)

// Interface compliance: exercised without a real pool. Behavior coverage lives
// in the conversation usecase tests (which use an in-memory fake repo).
func TestNewAugurConversationRepository_ImplementsInterface(t *testing.T) {
	var _ domain.AugurConversationRepository = NewAugurConversationRepository(nil)
}
