package retrieve_context_usecase

import (
	"alt/port/feed_search_port"
	"alt/port/rag_integration_port"
	"context"
	"fmt"
)

type RetrieveContextUsecase interface {
	Execute(ctx context.Context, query string) ([]rag_integration_port.RagContext, error)
}

type retrieveContextUsecase struct {
	searchFeedPort     feed_search_port.SearchFeedPort
	ragIntegrationPort rag_integration_port.RagIntegrationPort
}

func NewRetrieveContextUsecase(
	searchFeedPort feed_search_port.SearchFeedPort,
	ragIntegrationPort rag_integration_port.RagIntegrationPort,
) RetrieveContextUsecase {
	return &retrieveContextUsecase{
		searchFeedPort:     searchFeedPort,
		ragIntegrationPort: ragIntegrationPort,
	}
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, query string) ([]rag_integration_port.RagContext, error) {
	// 1. Get candidate articles from Meilisearch
	// We want enough candidates to ensure good overlap with vector search
	const candidateLimit = 50
	hits, _, err := u.searchFeedPort.SearchFeedsWithPagination(ctx, query, 0, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to search feeds for candidates: %w", err)
	}

	candidateIDs := make([]string, 0, len(hits))
	for _, hit := range hits {
		candidateIDs = append(candidateIDs, hit.ID)
	}

	// 2. Call RAG Retrieve Context
	contexts, err := u.ragIntegrationPort.RetrieveContext(ctx, query, candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve context from rag: %w", err)
	}

	return contexts, nil
}
