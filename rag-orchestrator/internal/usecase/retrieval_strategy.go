package usecase

import "context"

// RetrievalStrategy defines how context is retrieved for a given intent type.
type RetrievalStrategy interface {
	Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error)
	Name() string
}
