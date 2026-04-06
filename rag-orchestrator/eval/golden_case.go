package eval

// GoldenCase defines a single evaluation case with expected behavior.
type GoldenCase struct {
	ID                  string            `json:"id"`
	Query               string            `json:"query"`
	ConversationHistory []HistoryMessage   `json:"conversation_history,omitempty"`
	ArticleScope        *ArticleScopeInfo  `json:"article_scope,omitempty"`
	Expected            ExpectedBehavior   `json:"expected"`
	Tags                []string           `json:"tags,omitempty"` // e.g. "causal", "follow-up", "cjk"
}

// HistoryMessage represents a single message in the conversation history.
type HistoryMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
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
	ExpectedTopicKeywords []string `json:"expected_topic_keywords"`           // keywords that should appear in retrieved chunks
	RetrievalScope        string   `json:"retrieval_scope"`                   // "global", "article_only", "tool_only"
	MinRelevantContexts   int      `json:"min_relevant_contexts,omitempty"`   // minimum number of relevant chunks
	IrrelevantTitles      []string `json:"irrelevant_titles,omitempty"`       // titles that must NOT appear

	// Planning expectations
	ShouldClarify       bool   `json:"should_clarify"`
	ExpectedIntent      string `json:"expected_intent,omitempty"`       // expected intent classification
	ExpectedAnswerFormat string `json:"expected_answer_format,omitempty"` // "causal_analysis", "summary", etc.

	// Generation expectations
	MinAnswerLength    int      `json:"min_answer_length,omitempty"`    // minimum rune count
	RequiresCitations  bool     `json:"requires_citations"`
	ExpectedEntities   []string `json:"expected_entities,omitempty"`   // entities that should appear in answer
	ForbiddenPatterns  []string `json:"forbidden_patterns,omitempty"` // patterns that must NOT appear
}

// EvalResult holds the actual output from a single evaluation run.
type EvalResult struct {
	CaseID string `json:"case_id"`

	// Retrieval
	RetrievedTitles   []string  `json:"retrieved_titles"`
	RetrievedScores   []float32 `json:"retrieved_scores"`
	BM25HitCount      int       `json:"bm25_hit_count"`
	ExpandedQueries   []string  `json:"expanded_queries"`

	// Planning
	IntentClassified   string  `json:"intent_classified"`
	RetrievalPolicy    string  `json:"retrieval_policy"`
	PlannerConfidence  float64 `json:"planner_confidence"`
	ClarificationAsked bool    `json:"clarification_asked"`

	// Generation
	Answer         string   `json:"answer"`
	AnswerLength   int      `json:"answer_length"` // rune count
	CitationCount  int      `json:"citation_count"`
	CitedTitles    []string `json:"cited_titles"`
	IsFallback     bool     `json:"is_fallback"`
	FallbackReason string   `json:"fallback_reason,omitempty"`
	QualityFlags   []string `json:"quality_flags,omitempty"`
}

// CaseVerdict represents the pass/fail judgment for a single case.
type CaseVerdict struct {
	CaseID  string   `json:"case_id"`
	Passed  bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"` // human-readable failure reasons
}

// EvalReport summarizes the full evaluation run.
type EvalReport struct {
	Timestamp    string        `json:"timestamp"`
	CaseCount    int           `json:"case_count"`
	PassCount    int           `json:"pass_count"`
	FailCount    int           `json:"fail_count"`
	Verdicts     []CaseVerdict `json:"verdicts"`
	Metrics      AggregateMetrics `json:"metrics"`
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
	MeanFaithfulness       float64 `json:"mean_faithfulness"`
	MeanCitationCorrectness float64 `json:"mean_citation_correctness"`
	UnsupportedClaimRate   float64 `json:"unsupported_claim_rate"`
	FallbackRate           float64 `json:"fallback_rate"`
}
