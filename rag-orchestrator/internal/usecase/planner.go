package usecase

import (
	"fmt"
	"strings"

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

// clarificationThreshold is the minimum topic confidence needed to skip clarification
// for ambiguous follow-ups.
const clarificationThreshold = 0.5

// Plan resolves the user's query into a structured PlannerOutput.
// Steps: reference resolution → operation classification → retrieval policy → clarification check.
func (p *ConversationPlanner) Plan(
	query string,
	intent QueryIntent,
	state *domain.ConversationState,
	history []domain.Message,
) *domain.PlannerOutput {
	out := &domain.PlannerOutput{
		ArticleScopeAction: domain.ScopeKeep,
		Confidence:         0.5,
	}

	// Step 1: Detect topic shift early
	if isTopicShift(query) {
		out.Operation = domain.OpTopicShift
		out.RetrievalPolicy = domain.PolicyGlobalOnly
		out.ArticleScopeAction = domain.ScopeDrop
		out.Confidence = 0.9
		return out
	}

	// Step 2: Check if query is ambiguous and needs reference resolution
	ambiguous := isAmbiguousFollowUp(query)

	if ambiguous {
		return p.resolveAmbiguous(query, intent, state, history, out)
	}

	// Step 3: Map from SubIntentType or IntentType to operation
	return p.classifyExplicit(intent, state, out)
}

// resolveAmbiguous handles vague follow-ups like "もっと詳しく", "それって本当？".
func (p *ConversationPlanner) resolveAmbiguous(
	query string,
	intent QueryIntent,
	state *domain.ConversationState,
	history []domain.Message,
	out *domain.PlannerOutput,
) *domain.PlannerOutput {
	// No state = cannot resolve reference.
	// But if conversation history exists, the user is in an active dialogue —
	// fall back to OpDetail instead of demanding clarification.
	if state == nil {
		if len(history) > 0 {
			out.Operation = domain.OpDetail
			if intent.ArticleID != "" {
				out.RetrievalPolicy = domain.PolicyArticleOnly
			} else {
				out.RetrievalPolicy = domain.PolicyGlobalOnly
			}
			out.Confidence = 0.5
			return out
		}
		out.Operation = domain.OpClarify
		out.RetrievalPolicy = domain.PolicyNoRetrieval
		out.NeedsClarification = true
		out.ClarificationMsg = "何について詳しく知りたいですか？"
		out.Confidence = 0.3
		return out
	}

	// Low confidence = too ambiguous even with state → clarify
	if state.TopicConfidence < clarificationThreshold {
		out.Operation = domain.OpClarify
		out.RetrievalPolicy = domain.PolicyNoRetrieval
		out.NeedsClarification = true
		out.ClarificationMsg = buildClarificationMessage(state)
		out.Confidence = 0.4
		return out
	}

	// Resolve based on query pattern + state
	lower := strings.ToLower(query)

	if isFactCheckQuery(query, lower) {
		out.Operation = domain.OpFactCheck
		out.RetrievalPolicy = domain.PolicyArticlePlusGlobal
		out.Confidence = 0.7
		if len(state.FocusClaims) > 0 {
			out.ClaimFocus = state.FocusClaims[:1]
		}
		return out
	}

	if isDifferentPerspective(query) {
		out.Operation = domain.OpCritique
		out.RetrievalPolicy = domain.PolicyArticleOnly
		out.Confidence = 0.7
		return out
	}

	// Default ambiguous = detail (most common intent for "もっと詳しく")
	out.Operation = domain.OpDetail
	if intent.ArticleID != "" {
		out.RetrievalPolicy = domain.PolicyArticleOnly
	} else {
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	}
	out.Confidence = 0.65
	return out
}

// classifyExplicit maps explicit SubIntentType/IntentType to PlannerOutput.
func (p *ConversationPlanner) classifyExplicit(
	intent QueryIntent,
	state *domain.ConversationState,
	out *domain.PlannerOutput,
) *domain.PlannerOutput {
	out.Confidence = 0.85

	// SubIntentType takes priority (already classified by QueryClassifier)
	if intent.SubIntentType != SubIntentNone {
		return p.fromSubIntent(intent, out)
	}

	// IntentType classification
	return p.fromIntentType(intent, out)
}

func (p *ConversationPlanner) fromSubIntent(intent QueryIntent, out *domain.PlannerOutput) *domain.PlannerOutput {
	switch intent.SubIntentType {
	case SubIntentDetail:
		out.Operation = domain.OpDetail
		out.RetrievalPolicy = domain.PolicyArticleOnly
	case SubIntentEvidence:
		out.Operation = domain.OpEvidence
		out.RetrievalPolicy = domain.PolicyArticleOnly
	case SubIntentRelatedArticles:
		out.Operation = domain.OpRelatedArticles
		out.RetrievalPolicy = domain.PolicyToolOnly
	case SubIntentCritique:
		out.Operation = domain.OpCritique
		out.RetrievalPolicy = domain.PolicyArticleOnly
	case SubIntentOpinion:
		out.Operation = domain.OpOpinion
		out.RetrievalPolicy = domain.PolicyArticleOnly
	case SubIntentImplication:
		out.Operation = domain.OpImplication
		out.RetrievalPolicy = domain.PolicyArticlePlusGlobal
	case SubIntentSummaryRefresh:
		out.Operation = domain.OpSummaryRefresh
		out.RetrievalPolicy = domain.PolicyArticleOnly
	default:
		out.Operation = domain.OpGeneral
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	}
	return out
}

func (p *ConversationPlanner) fromIntentType(intent QueryIntent, out *domain.PlannerOutput) *domain.PlannerOutput {
	switch intent.IntentType {
	case IntentArticleScoped:
		out.Operation = domain.OpGeneral
		out.RetrievalPolicy = domain.PolicyArticleOnly
	case IntentComparison:
		out.Operation = domain.OpCompare
		out.RetrievalPolicy = domain.PolicyArticlePlusGlobal
	case IntentCausalExplanation:
		out.Operation = domain.OpCausalExplanation
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	case IntentTemporal:
		out.Operation = domain.OpGeneral
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	case IntentTopicDeepDive:
		out.Operation = domain.OpDetail
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	case IntentFactCheck:
		out.Operation = domain.OpFactCheck
		out.RetrievalPolicy = domain.PolicyArticlePlusGlobal
	default:
		out.Operation = domain.OpGeneral
		out.RetrievalPolicy = domain.PolicyGlobalOnly
	}
	return out
}

// --- Pattern Matching Helpers ---

// isAmbiguousFollowUp detects vague follow-up queries that need reference resolution.
// A query is ambiguous only if it consists primarily of a pattern with no substantive
// additional content. "もっと詳しく" is ambiguous, but "PyO3について詳しく教えて" is not.
func isAmbiguousFollowUp(query string) bool {
	lower := strings.ToLower(query)
	ambiguousPatterns := []string{
		"もっと詳しく", "詳しく教えて", "もう少し",
		"それって本当", "本当に", "それは事実",
		"別の観点", "他の視点", "別の見方",
		"tell me more", "more detail", "is that true", "different perspective",
	}
	for _, p := range ambiguousPatterns {
		if strings.Contains(lower, p) || strings.Contains(query, p) {
			// Check if the query has substantive content beyond the pattern.
			// Remove the pattern and see if meaningful content remains.
			remainder := strings.Replace(query, p, "", 1)
			remainder = strings.TrimSpace(remainder)
			// Strip common particles and punctuation
			for _, strip := range []string{"？", "?", "。", ".", "、", "の", "について", "は", "を", "に"} {
				remainder = strings.ReplaceAll(remainder, strip, "")
			}
			remainder = strings.TrimSpace(remainder)
			// If after removing the pattern and particles, less than 2 runes remain,
			// the query is truly ambiguous (just the pattern).
			if len([]rune(remainder)) < 2 {
				return true
			}
			// Has substantive content — not ambiguous.
			return false
		}
	}
	return false
}

// isTopicShift detects explicit topic change signals.
func isTopicShift(query string) bool {
	lower := strings.ToLower(query)
	patterns := []string{
		"別件", "話題を変えて", "別の話", "ここからは別",
		"different topic", "change the subject", "new topic",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) || strings.Contains(query, p) {
			return true
		}
	}
	return false
}

func isFactCheckQuery(query, lower string) bool {
	patterns := []string{"本当", "事実", "それって本当", "is that true", "is it true"}
	for _, p := range patterns {
		if strings.Contains(lower, p) || strings.Contains(query, p) {
			return true
		}
	}
	return false
}

func isDifferentPerspective(query string) bool {
	patterns := []string{"別の観点", "他の視点", "別の見方", "different perspective"}
	lower := strings.ToLower(query)
	for _, p := range patterns {
		if strings.Contains(lower, p) || strings.Contains(query, p) {
			return true
		}
	}
	return false
}

func buildClarificationMessage(state *domain.ConversationState) string {
	if len(state.FocusEntities) == 0 {
		return "何を詳しく知りたいですか？"
	}
	entities := strings.Join(state.FocusEntities, " / ")
	return fmt.Sprintf("何を詳しく知りたいですか？ 直前の回答では %s を扱いました", entities)
}
