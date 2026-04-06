package domain

import "context"

// QueryPlan is the structured output from the news-creator PlanQuery endpoint.
// It replaces the rule-based ConversationPlanner and query expansion.
type QueryPlan struct {
	ResolvedQuery    string   // Self-contained search query with coreferences resolved
	SearchQueries    []string // Expanded search queries (3-5, mixed Japanese/English)
	Intent           string   // causal_explanation, temporal, synthesis, comparison, fact_check, topic_deep_dive, general
	RetrievalPolicy  string   // global_only, article_only, tool_only, no_retrieval
	AnswerFormat     string   // causal_analysis, summary, list, detail, comparison, fact_check
	ShouldClarify    bool
	ClarificationMsg string
	TopicEntities    []string
}

// QueryPlannerInput is the input for the query planner.
type QueryPlannerInput struct {
	Query               string
	ConversationHistory []Message
	ArticleID           string
	ArticleTitle        string
	LastAnswerScope     string
}

// QueryPlannerPort defines the interface for LLM-based query planning.
// Implemented by the news-creator PlanQuery client.
type QueryPlannerPort interface {
	PlanQuery(ctx context.Context, input QueryPlannerInput) (*QueryPlan, error)
}
