package domain

// ConversationMode classifies the current conversation style.
type ConversationMode string

const (
	ModeArticleScoped ConversationMode = "article_scoped"
	ModeOpenTopic     ConversationMode = "open_topic"
	ModeFactCheck     ConversationMode = "fact_check"
	ModeDiscovery     ConversationMode = "discovery"
)

// AnswerScope describes the type of the last answer provided.
type AnswerScope string

const (
	ScopeSummary         AnswerScope = "summary"
	ScopeDetail          AnswerScope = "detail"
	ScopeEvidence        AnswerScope = "evidence"
	ScopeRelatedArticles AnswerScope = "related_articles"
	ScopeCritique        AnswerScope = "critique"
	ScopeOpinion         AnswerScope = "opinion"
	ScopeImplication     AnswerScope = "implication"
)

// ConversationState holds the durable state of a conversation thread.
// Updated after every successful answer to enable stateful follow-up handling.
type ConversationState struct {
	ThreadID            string
	Mode                ConversationMode
	CurrentTopic        string
	CurrentArticleID    string
	CurrentArticleTitle string
	LastAnswerScope     AnswerScope
	FocusEntities       []string
	FocusClaims         []string
	LastCitations       []string
	TopicConfidence     float64
	TurnCount           int
}
