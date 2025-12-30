package domain

import (
	"time"

	"github.com/google/uuid"
)

// TopicSummary represents a summarized topic from recent news
type TopicSummary struct {
	Topic       string       `json:"topic"`
	Headline    string       `json:"headline"`
	Summary     string       `json:"summary"`
	Importance  float32      `json:"importance"` // 0-1 score
	ArticleRefs []ArticleRef `json:"article_refs"`
	Keywords    []string     `json:"keywords"`
}

// ArticleRef is a lightweight reference to a source article
type ArticleRef struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// MorningLetterResponse represents the parsed LLM response for morning letter topics
type MorningLetterResponse struct {
	Topics []TopicSummary `json:"topics"`
	Meta   TopicsMeta     `json:"meta"`
}

// TopicsMeta contains metadata about the topics analysis
type TopicsMeta struct {
	TopicsFound        int    `json:"topics_found"`
	CoverageAssessment string `json:"coverage_assessment"`
}
