package domain

import "github.com/google/uuid"

// KnowledgeHomeLensFilter is the canonical read-path filter resolved from a lens.
type KnowledgeHomeLensFilter struct {
	LensID     uuid.UUID
	TagNames   []string
	FeedIDs    []uuid.UUID
	TimeWindow string
}
