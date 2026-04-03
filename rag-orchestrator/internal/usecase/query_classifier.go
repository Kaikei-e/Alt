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

	// Causal/explanatory patterns (before Temporal — "最近の真因" is causal, not temporal)
	if matchesCausal(query, lower) {
		return IntentCausalExplanation
	}

	// Synthesis patterns (before Temporal — "最近のNYと芸術のかかわり" is synthesis, not temporal)
	if matchesSynthesis(query, lower) {
		return IntentSynthesis
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

// ClassifySubIntent classifies the analytical sub-intent of a user question.
// Used for article-scoped queries where the question text has been extracted.
// Returns SubIntentNone if no specific analytical intent is detected.
// Priority: related_articles > evidence > detail > critique > opinion > implication > summary_refresh.
// Pure function — does not use LLM or context.
func (c *QueryClassifier) ClassifySubIntent(query string) SubIntentType {
	lower := strings.ToLower(query)

	if matchesRelatedArticles(query, lower) {
		return SubIntentRelatedArticles
	}
	if matchesEvidence(query, lower) {
		return SubIntentEvidence
	}
	if matchesDetail(query, lower) {
		return SubIntentDetail
	}
	if matchesCritique(query, lower) {
		return SubIntentCritique
	}
	if matchesOpinion(query, lower) {
		return SubIntentOpinion
	}
	if matchesImplication(query, lower) {
		return SubIntentImplication
	}
	if matchesSummaryRefresh(query, lower) {
		return SubIntentSummaryRefresh
	}
	return SubIntentNone
}

func matchesCritique(query, lower string) bool {
	jpKeywords := []string{"反論", "批判", "弱点", "問題点", "欠点", "リスク", "デメリット", "懸念", "課題", "限界"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"counterargument", "criticism", "weakness", "limitation", "drawback", "risk", "concern", "flaw", "downside"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesOpinion(query, lower string) bool {
	jpKeywords := []string{"どう思う", "評価", "意見", "見解", "感想", "判断"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"what do you think", "opinion", "assessment", "evaluation", "judgment", "your view"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesImplication(query, lower string) bool {
	jpKeywords := []string{"影響は", "意味は", "どういう意味", "結果は", "将来", "今後"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"implication", "what does this mean", "impact", "consequence", "going forward"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesRelatedArticles(query, lower string) bool {
	jpKeywords := []string{"関連する記事", "似た記事", "関連記事", "他にもある"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"related articles", "similar articles", "related stories"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesEvidence(query, lower string) bool {
	jpKeywords := []string{"根拠", "エビデンス", "証拠", "出典"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"evidence", "proof", "citation", "source of"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesDetail(query, lower string) bool {
	jpKeywords := []string{"技術的", "詳細", "具体例", "仕組み", "メカニズム"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"technical", "detail", "specific example", "mechanism", "how does it work"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesSummaryRefresh(query, lower string) bool {
	jpKeywords := []string{"結論だけ", "もう一度", "要約して", "まとめ直して"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"just the conclusion", "summarize again", "recap"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesCausal(query, lower string) bool {
	jpKeywords := []string{"真因", "原因", "要因", "なぜ", "理由", "根源"}
	for _, kw := range jpKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	enKeywords := []string{"root cause", "why did", "reason behind", "caused by", "what caused"}
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func matchesSynthesis(query, lower string) bool {
	// 1. Explicit synthesis signals (Japanese)
	jpSynthKeywords := []string{"そもそも", "全体像", "概観", "歴史的"}
	for _, kw := range jpSynthKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}

	// 2. "Xとは何か" pattern
	if strings.Contains(query, "とは何") {
		return true
	}

	// 3. Abstract relationship pattern: <Entity>と<Entity>の(かかわり|関係|影響|つながり|関連|関係性)
	relationWords := []string{"かかわり", "関係", "つながり", "関連", "関係性"}
	hasRelationWord := false
	for _, rw := range relationWords {
		if strings.Contains(query, rw) {
			hasRelationWord = true
			break
		}
	}
	if hasRelationWord && strings.Contains(query, "と") {
		return true
	}

	// 4. "影響" with "全体" or "と" pattern (broad impact, not article-scoped implication)
	if strings.Contains(query, "影響") && (strings.Contains(query, "全体") || strings.Contains(query, "と")) {
		return true
	}

	// 5. English synthesis patterns
	enSynthPatterns := []string{"relationship between", "overview of", "how are", "connected"}
	for _, pat := range enSynthPatterns {
		if strings.Contains(lower, pat) {
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
