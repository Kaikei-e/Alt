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
	next := &domain.ConversationState{
		ThreadID: threadID,
	}

	// Carry forward from previous state
	prevTurnCount := 0
	if prev != nil {
		prevTurnCount = prev.TurnCount
		next.FocusEntities = prev.FocusEntities
		next.FocusClaims = prev.FocusClaims
		next.CurrentTopic = prev.CurrentTopic
	}
	next.TurnCount = prevTurnCount + 1

	// Determine mode from intent
	switch intent.IntentType {
	case IntentArticleScoped:
		next.Mode = domain.ModeArticleScoped
		next.CurrentArticleID = intent.ArticleID
		next.CurrentArticleTitle = intent.ArticleTitle
	case IntentFactCheck:
		next.Mode = domain.ModeFactCheck
	default:
		next.Mode = domain.ModeOpenTopic
	}

	// Handle topic shift from planner
	if plannerOutput != nil && plannerOutput.Operation == domain.OpTopicShift {
		next.Mode = domain.ModeOpenTopic
		next.CurrentArticleID = ""
		next.CurrentArticleTitle = ""
		next.FocusEntities = nil
		next.FocusClaims = nil
	}

	// Map planner operation to answer scope
	next.LastAnswerScope = operationToScope(plannerOutput)

	// Extract citations from answer
	if answerOutput != nil {
		citations := make([]string, 0, len(answerOutput.Citations))
		for _, c := range answerOutput.Citations {
			citations = append(citations, c.ChunkID)
		}
		next.LastCitations = citations
	}

	// Topic confidence: same article = high, same mode = medium
	next.TopicConfidence = computeTopicConfidence(prev, next)

	return next
}

func operationToScope(plan *domain.PlannerOutput) domain.AnswerScope {
	if plan == nil {
		return domain.ScopeSummary
	}
	switch plan.Operation {
	case domain.OpDetail:
		return domain.ScopeDetail
	case domain.OpEvidence:
		return domain.ScopeEvidence
	case domain.OpRelatedArticles:
		return domain.ScopeRelatedArticles
	case domain.OpCritique:
		return domain.ScopeCritique
	case domain.OpOpinion:
		return domain.ScopeOpinion
	case domain.OpImplication:
		return domain.ScopeImplication
	default:
		return domain.ScopeSummary
	}
}

func computeTopicConfidence(prev, next *domain.ConversationState) float64 {
	if prev == nil {
		return 0.5 // First turn: neutral confidence
	}
	confidence := 0.5
	if prev.CurrentArticleID != "" && prev.CurrentArticleID == next.CurrentArticleID {
		confidence += 0.3 // Same article scope
	}
	if prev.Mode == next.Mode {
		confidence += 0.2 // Same conversation mode
	}
	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}
