package eval

// Case categories. A case belongs to exactly one; the aggregate report breaks
// pass rates down by category so a regression can be attributed to a stage
// rather than to "the eval got worse".
const (
	CategoryRecallMiss    = "recall-miss"
	CategoryCrossLingual  = "cross-lingual"
	CategoryArticleScoped = "article-scoped"
	CategoryTemporal      = "temporal"
	CategoryCausal        = "causal"
	CategoryComparison    = "comparison"
	CategoryFactCheck     = "fact-check"
	CategoryFollowUp      = "follow-up"
	CategorySynthesis     = "synthesis"
	CategoryDeepDive      = "deep-dive"
	CategoryNoAnswer      = "no-answer"
	CategoryIndexHygiene  = "index-hygiene"
	CategoryRegression    = "regression"
)

// KnownCategories bounds the category vocabulary so a typo in the golden file
// cannot silently create a category nobody reads.
var KnownCategories = []string{
	CategoryRecallMiss,
	CategoryCrossLingual,
	CategoryArticleScoped,
	CategoryTemporal,
	CategoryCausal,
	CategoryComparison,
	CategoryFactCheck,
	CategoryFollowUp,
	CategorySynthesis,
	CategoryDeepDive,
	CategoryNoAnswer,
	CategoryIndexHygiene,
	CategoryRegression,
}

// LanguageMixed marks a case whose ground truth spans both languages. Such a
// case is answerable without crossing the language boundary, so it is not a
// controlled cross-lingual probe.
const LanguageMixed = "mixed"

// LanguagePair records the language the user typed in and the language of the
// articles a correct retrieval has to reach. When they differ the case only
// passes if the retriever crosses the language boundary.
type LanguagePair struct {
	Query  string `json:"query,omitempty"`
	Corpus string `json:"corpus,omitempty"`
}

// GoldenCase defines a single evaluation case with expected behavior.
type GoldenCase struct {
	ID                  string            `json:"id"`
	Query               string            `json:"query"`
	Category            string            `json:"category"`
	Language            LanguagePair      `json:"language,omitempty"`
	ConversationHistory []HistoryMessage  `json:"conversation_history,omitempty"`
	ArticleScope        *ArticleScopeInfo `json:"article_scope,omitempty"`
	Expected            ExpectedBehavior  `json:"expected"`
	Tags                []string          `json:"tags,omitempty"` // e.g. "causal", "follow-up", "cjk"
	Note                string            `json:"note,omitempty"` // why this case exists, in one line
}

// IsCrossLingual reports whether answering the case requires retrieving
// articles written in a different language from the query. Mixed-language
// ground truth does not count: it can be satisfied without crossing.
func (gc GoldenCase) IsCrossLingual() bool {
	if gc.Language.Query == "" || gc.Language.Corpus == "" {
		return false
	}
	if gc.Language.Query == LanguageMixed || gc.Language.Corpus == LanguageMixed {
		return false
	}
	return gc.Language.Query != gc.Language.Corpus
}

// HistoryMessage represents a single message in the conversation history.
type HistoryMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// ArticleScopeInfo identifies an article that scopes the query.
type ArticleScopeInfo struct {
	ArticleID string `json:"article_id"`
	Title     string `json:"title"`
}

// ExpectedBehavior defines what a correct response looks like.
type ExpectedBehavior struct {
	// Retrieval expectations
	ExpectedTopicKeywords []string `json:"expected_topic_keywords"`         // keywords that should appear in retrieved chunks
	RetrievalScope        string   `json:"retrieval_scope"`                 // "global", "article_only", "tool_only"
	MinRelevantContexts   int      `json:"min_relevant_contexts,omitempty"` // minimum number of relevant chunks
	IrrelevantTitles      []string `json:"irrelevant_titles,omitempty"`     // titles that must NOT appear

	// Article-level ground truth, verified against rag_documents. These drive
	// the retrieval-stage metrics; the keyword lists above stay for the older
	// cases and for answer-text checks.
	RelevantArticleIDs         []string `json:"relevant_article_ids,omitempty"`          // qrels: articles a correct retrieval reaches
	ExpectedCitationArticleIDs []string `json:"expected_citation_article_ids,omitempty"` // subset that must be cited
	ForbiddenArticleIDs        []string `json:"forbidden_article_ids,omitempty"`         // spam / duplicates / index pollution
	MinExpectedRecall          float64  `json:"min_expected_recall,omitempty"`           // recall@20 floor over RelevantArticleIDs

	// Planning expectations
	ShouldClarify        bool   `json:"should_clarify"`
	ExpectedIntent       string `json:"expected_intent,omitempty"`        // expected intent classification
	ExpectedAnswerFormat string `json:"expected_answer_format,omitempty"` // "causal_analysis", "summary", etc.

	// Generation expectations
	MinAnswerLength   int      `json:"min_answer_length,omitempty"` // minimum rune count
	RequiresCitations bool     `json:"requires_citations"`
	ExpectNoAnswer    bool     `json:"expect_no_answer,omitempty"`   // corpus has nothing: silence beats invention
	ExpectedEntities  []string `json:"expected_entities,omitempty"`  // entities that should appear in answer
	ForbiddenPatterns []string `json:"forbidden_patterns,omitempty"` // patterns that must NOT appear
	ExpectedStructure []string `json:"expected_structure,omitempty"` // markdown headers/keywords that must appear in answer
}

// RelevanceGrades derives graded relevance for nDCG from the article-level
// ground truth: articles that must be cited are graded above articles that are
// merely acceptable context.
func (e ExpectedBehavior) RelevanceGrades() map[string]int {
	grades := make(map[string]int, len(e.RelevantArticleIDs)+len(e.ExpectedCitationArticleIDs))
	for _, id := range e.RelevantArticleIDs {
		grades[id] = 1
	}
	for _, id := range e.ExpectedCitationArticleIDs {
		grades[id] = 2
	}
	return grades
}

// EvalResult holds the actual output from a single evaluation run.
type EvalResult struct {
	CaseID string `json:"case_id"`

	// Retrieval
	RetrievedTitles []string  `json:"retrieved_titles"`
	RetrievedScores []float32 `json:"retrieved_scores"`
	BM25HitCount    int       `json:"bm25_hit_count"`
	ExpandedQueries []string  `json:"expanded_queries"`

	// Article-level retrieval trace. RetrievedArticleIDs is the final order the
	// generator saw; PreRerankArticleIDs is the same set ordered by fusion score,
	// which is what the rerank delta is measured against.
	RetrievedArticleIDs []string `json:"retrieved_article_ids,omitempty"`
	PreRerankArticleIDs []string `json:"pre_rerank_article_ids,omitempty"`
	RerankApplied       bool     `json:"rerank_applied,omitempty"`

	// Planning
	IntentClassified   string  `json:"intent_classified"`
	RetrievalPolicy    string  `json:"retrieval_policy"`
	PlannerConfidence  float64 `json:"planner_confidence"`
	ClarificationAsked bool    `json:"clarification_asked"`

	// Generation
	Answer           string   `json:"answer"`
	AnswerLength     int      `json:"answer_length"` // rune count
	CitationCount    int      `json:"citation_count"`
	CitedTitles      []string `json:"cited_titles"`
	CitedArticleIDs  []string `json:"cited_article_ids,omitempty"`
	IsFallback       bool     `json:"is_fallback"`
	FallbackReason   string   `json:"fallback_reason,omitempty"`
	QualityFlags     []string `json:"quality_flags,omitempty"`
	PromptTokenCount int      `json:"prompt_token_count,omitempty"` // measured prompt size in tokens
}

// CaseVerdict represents the pass/fail judgment for a single case.
type CaseVerdict struct {
	CaseID   string   `json:"case_id"`
	Category string   `json:"category,omitempty"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"` // human-readable failure reasons
}

// EvalReport summarizes the full evaluation run.
type EvalReport struct {
	Timestamp  string                     `json:"timestamp"`
	Profile    ProfileSummary             `json:"profile"`
	CaseCount  int                        `json:"case_count"`
	PassCount  int                        `json:"pass_count"`
	FailCount  int                        `json:"fail_count"`
	Verdicts   []CaseVerdict              `json:"verdicts"`
	Metrics    AggregateMetrics           `json:"metrics"`
	Stages     StageMetrics               `json:"stages"`
	Categories map[string]CategorySummary `json:"categories,omitempty"`
}

// CategorySummary is the pass rate for one case category.
type CategorySummary struct {
	CaseCount int `json:"case_count"`
	PassCount int `json:"pass_count"`
}

// StageMetrics scores retrieval, rerank and generation separately so a change
// can be attributed to the stage that caused it.
type StageMetrics struct {
	Retrieval  RetrievalMetrics  `json:"retrieval"`
	Rerank     RerankMetrics     `json:"rerank"`
	Generation GenerationMetrics `json:"generation"`
}

// RetrievalMetrics scores the retrieval stage against article-level ground truth.
type RetrievalMetrics struct {
	CaseCount        int     `json:"case_count"`
	MeanRecallAt5    float64 `json:"mean_recall_at_5"`
	MeanRecallAt10   float64 `json:"mean_recall_at_10"`
	MeanRecallAt20   float64 `json:"mean_recall_at_20"`
	MeanNDCGAt10     float64 `json:"mean_ndcg_at_10"`
	MeanMRR          float64 `json:"mean_mrr"`
	BM25ZeroRate     float64 `json:"bm25_zero_rate"`
	ForbiddenHitRate float64 `json:"forbidden_hit_rate"`
}

// RerankMetrics scores the reranker as the nDCG@10 it adds on top of the
// fusion order it was handed.
type RerankMetrics struct {
	CaseCount          int     `json:"case_count"`
	AppliedRate        float64 `json:"applied_rate"`
	MeanNDCGAt10Before float64 `json:"mean_ndcg_at_10_before"`
	MeanNDCGAt10After  float64 `json:"mean_ndcg_at_10_after"`
	MeanNDCGAt10Delta  float64 `json:"mean_ndcg_at_10_delta"`
}

// GenerationMetrics scores the answer stage.
type GenerationMetrics struct {
	CaseCount               int     `json:"case_count"`
	MeanFaithfulness        float64 `json:"mean_faithfulness"`
	MeanCitationCorrectness float64 `json:"mean_citation_correctness"`
	CitationRecall          float64 `json:"citation_recall"`
	ForbiddenCitationRate   float64 `json:"forbidden_citation_rate"`
	NoAnswerHonestyRate     float64 `json:"no_answer_honesty_rate"`
	FallbackRate            float64 `json:"fallback_rate"`
}

// AggregateMetrics holds the aggregate scores across all cases.
type AggregateMetrics struct {
	// Retrieval
	MeanRecallAt20    float64 `json:"mean_recall_at_20"`
	MeanNDCGAt10      float64 `json:"mean_ndcg_at_10"`
	MeanTop1Precision float64 `json:"mean_top1_precision"`
	BM25ZeroRate      float64 `json:"bm25_zero_rate"` // fraction of queries with 0 BM25 hits

	// Planning
	FollowUpResolutionRate float64 `json:"follow_up_resolution_rate"`
	ClarificationPrecision float64 `json:"clarification_precision"`
	IntentAccuracy         float64 `json:"intent_accuracy"`

	// Generation
	MeanFaithfulness        float64 `json:"mean_faithfulness"`
	MeanCitationCorrectness float64 `json:"mean_citation_correctness"`
	UnsupportedClaimRate    float64 `json:"unsupported_claim_rate"`
	FallbackRate            float64 `json:"fallback_rate"`

	// Instruction adherence
	InstructionAdherenceRate float64 `json:"instruction_adherence_rate"` // fraction of cases where answer matches expected structure
	MeanPromptTokens         float64 `json:"mean_prompt_tokens"`         // average prompt token count across cases
}
