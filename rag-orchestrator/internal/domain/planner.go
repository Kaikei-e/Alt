package domain

// PlannerOperation classifies what the system should do with the user's query.
type PlannerOperation string

const (
	OpDetail            PlannerOperation = "detail"
	OpEvidence          PlannerOperation = "evidence"
	OpRelatedArticles   PlannerOperation = "related_articles"
	OpClarify           PlannerOperation = "clarify"
	OpCompare           PlannerOperation = "compare"
	OpTopicShift        PlannerOperation = "topic_shift"
	OpFactCheck         PlannerOperation = "fact_check"
	OpSummaryRefresh    PlannerOperation = "summary_refresh"
	OpCritique          PlannerOperation = "critique"
	OpOpinion           PlannerOperation = "opinion"
	OpImplication       PlannerOperation = "implication"
	OpGeneral           PlannerOperation = "general"
	OpCausalExplanation PlannerOperation = "causal_explanation"
)

// ArticleScopeAction determines how article scope should change.
type ArticleScopeAction string

const (
	ScopeKeep   ArticleScopeAction = "keep"
	ScopeDrop   ArticleScopeAction = "drop"
	ScopeSwitch ArticleScopeAction = "switch"
)

// RetrievalPolicy determines the retrieval strategy for a query.
type RetrievalPolicy string

const (
	PolicyArticleOnly       RetrievalPolicy = "article_only"
	PolicyArticlePlusGlobal RetrievalPolicy = "article_plus_global"
	PolicyGlobalOnly        RetrievalPolicy = "global_only"
	PolicyToolOnly          RetrievalPolicy = "tool_only"
	PolicyNoRetrieval       RetrievalPolicy = "no_retrieval"
)

// PlannerOutput is the structured result of conversation planning.
// Produced before retrieval to determine what operation, retrieval policy,
// and clarification behavior the system should use.
type PlannerOutput struct {
	Operation          PlannerOperation
	Topic              string
	EntityFocus        []string
	ClaimFocus         []string
	ArticleScopeAction ArticleScopeAction
	RetrievalPolicy    RetrievalPolicy
	NeedsClarification bool
	ClarificationMsg   string
	Confidence         float64
}
