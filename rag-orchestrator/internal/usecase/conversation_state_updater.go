package usecase

import (
	"rag-orchestrator/internal/domain"
)

// DeriveStateUpdate computes the next conversation state from the previous state,
// the resolved intent, the planner output, and the completed answer.
// Returns a new state (immutable update). prev may be nil for the first turn.
func DeriveStateUpdate(
	prev *domain.ConversationState,
	threadID string,
	intent QueryIntent,
	plannerOutput *domain.PlannerOutput,
	answerOutput *AnswerWithRAGOutput,
) *domain.ConversationState {
	panic("not implemented")
}
