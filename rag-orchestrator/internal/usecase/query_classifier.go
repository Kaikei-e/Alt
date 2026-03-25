package usecase

import (
	"context"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
)

// QueryClassifier classifies user queries into intent types.
// Uses a hybrid approach: rule-based first, optional LLM fallback for ambiguous cases.
type QueryClassifier struct {
	llmClient  domain.LLMClient
	llmTimeout time.Duration
}

// NewQueryClassifier creates a new classifier.
// llmClient can be nil to disable LLM fallback.
// llmTimeout is the timeout for LLM classification (0 = no LLM fallback).
func NewQueryClassifier(llmClient domain.LLMClient, llmTimeout time.Duration) *QueryClassifier {
	return &QueryClassifier{
		llmClient:  llmClient,
		llmTimeout: llmTimeout,
	}
}

// Classify determines the intent type of a query.
// Step 1: Rule-based keyword matching (0ms).
// Step 2: LLM fallback for ambiguous queries (if enabled).
func (c *QueryClassifier) Classify(ctx context.Context, query string) IntentType {
	// Step 0: Check article-scoped (reuse existing parser)
	parsed := ParseQueryIntent(query)
	if parsed.IntentType == IntentArticleScoped {
		return IntentArticleScoped
	}

	lower := strings.ToLower(query)

	// Step 1: Rule-based classification

	// Comparison patterns
	if matchesComparison(query, lower) {
		return IntentComparison
	}

	// Temporal patterns
	if matchesTemporal(query, lower) {
		return IntentTemporal
	}

	// FactCheck patterns (before DeepDive, since "本当" is more specific)
	if matchesFactCheck(query, lower) {
		return IntentFactCheck
	}

	// DeepDive patterns
	if matchesDeepDive(query, lower) {
		return IntentTopicDeepDive
	}

	// Step 2: LLM fallback (TODO: Phase 2 feature flag)
	// Currently disabled — always falls through to General

	return IntentGeneral
}

func matchesComparison(query, lower string) bool {
	// Japanese comparison keywords
	jpKeywords := []string{"違い", "比較", "対"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	// English comparison keywords
	enKeywords := []string{" vs ", " vs. ", "compare", "difference between", "compared to"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesTemporal(query, lower string) bool {
	jpKeywords := []string{"最近", "今週", "今日", "最新", "昨日", "先週"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"latest", "recent", "this week", "today", "yesterday", "last week"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesFactCheck(query, lower string) bool {
	jpKeywords := []string{"本当", "事実", "正しい"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"is it true", "fact check", "is it correct", "is it accurate"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesDeepDive(query, lower string) bool {
	jpKeywords := []string{"詳しく", "深掘り", "について教えて", "について詳しく"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"in detail", "explain", "tell me about", "deep dive"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
