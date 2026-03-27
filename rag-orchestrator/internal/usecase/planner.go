package usecase

import (
	"rag-orchestrator/internal/domain"
)

// ConversationPlanner resolves ambiguous user queries into structured planning output.
// Uses rule-based logic (not LLM) due to Gemma 3 4B constraints.
type ConversationPlanner struct {
	classifier *QueryClassifier
}

// NewConversationPlanner creates a planner that uses the existing QueryClassifier
// for intent and sub-intent classification.
func NewConversationPlanner(classifier *QueryClassifier) *ConversationPlanner {
	return &ConversationPlanner{classifier: classifier}
}

// Plan resolves the user's query into a structured PlannerOutput.
// Steps: reference resolution → operation classification → retrieval policy → clarification check.
func (p *ConversationPlanner) Plan(
	query string,
	intent QueryIntent,
	state *domain.ConversationState,
	history []domain.Message,
) *domain.PlannerOutput {
	panic("not implemented")
}
