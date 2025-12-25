package rag_adapter

import (
	"alt/gateway/rag_gateway"
	"alt/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/http"
)

type RagAdapter struct {
	client rag_gateway.ClientWithResponsesInterface
}

func NewRagAdapter(client rag_gateway.ClientWithResponsesInterface) rag_integration_port.RagIntegrationPort {
	return &RagAdapter{
		client: client,
	}
}

func (a *RagAdapter) UpsertArticle(ctx context.Context, input rag_integration_port.UpsertArticleInput) error {
	reqBody := rag_gateway.UpsertIndexJSONRequestBody{
		ArticleId:   input.ArticleID,
		Body:        input.Body,
		PublishedAt: input.PublishedAt,
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
