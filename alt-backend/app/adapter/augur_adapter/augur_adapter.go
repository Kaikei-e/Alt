package augur_adapter

import (
	"alt/gateway/rag_gateway"
	"alt/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/http"
	"time"
)

type AugurAdapter struct {
	client rag_gateway.ClientWithResponsesInterface
}

func NewAugurAdapter(client rag_gateway.ClientWithResponsesInterface) rag_integration_port.RagIntegrationPort {
	return &AugurAdapter{
		client: client,
	}
}

func (a *AugurAdapter) UpsertArticle(ctx context.Context, input rag_integration_port.UpsertArticleInput) error {
	var publishedAt time.Time
	if input.PublishedAt != nil {
		publishedAt = *input.PublishedAt
	}

	reqBody := rag_gateway.UpsertIndexJSONRequestBody{
		ArticleId:   input.ArticleID,
		Body:        input.Body,
		PublishedAt: publishedAt,
		Title:       input.Title,
		UpdatedAt:   input.UpdatedAt,
		Url:         input.URL,
		UserId:      input.UserID,
	}

	resp, err := a.client.UpsertIndexWithResponse(ctx, reqBody)
	if err != nil {
		return fmt.Errorf("failed to call UpsertIndex: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		logger.Logger.Error("RAG UpsertIndex failed", "status", resp.StatusCode(), "body", string(resp.Body))
		return fmt.Errorf("RAG UpsertIndex returned non-OK status: %d", resp.StatusCode())
	}

	return nil
}

func (a *AugurAdapter) RetrieveContext(ctx context.Context, query string, candidateIDs []string) ([]rag_integration_port.RagContext, error) {
	body := rag_gateway.RetrieveRequest{
		Query:               query,
		CandidateArticleIds: &candidateIDs,
	}

	resp, err := a.client.RetrieveContextWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("failed to call retrieve context: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("rag-orchestrator returned status %d", resp.StatusCode())
	}

	if resp.JSON200 == nil || resp.JSON200.Contexts == nil {
		return nil, fmt.Errorf("rag-orchestrator returned empty response")
	}

	var contexts []rag_integration_port.RagContext
	for _, c := range *resp.JSON200.Contexts {
		var ctxItem rag_integration_port.RagContext
		if c.ChunkText != nil {
			ctxItem.ChunkText = *c.ChunkText
		}
		if c.Url != nil {
			ctxItem.URL = *c.Url
		}
		if c.Title != nil {
			ctxItem.Title = *c.Title
		}
		if c.PublishedAt != nil {
			ctxItem.PublishedAt = c.PublishedAt
		}
		if c.Score != nil {
			ctxItem.Score = *c.Score
		}
		if c.DocumentVersion != nil {
			ctxItem.DocumentVersion = *c.DocumentVersion
		}
		contexts = append(contexts, ctxItem)
	}

	return contexts, nil
}
