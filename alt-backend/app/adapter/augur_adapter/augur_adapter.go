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

type RagClientInterface interface {
	rag_gateway.ClientWithResponsesInterface
	AnswerWithRAGStream(ctx context.Context, body rag_gateway.AnswerWithRAGStreamJSONRequestBody, reqEditors ...rag_gateway.RequestEditorFn) (*http.Response, error)
}

type AugurAdapter struct {
	client RagClientInterface
}

func NewAugurAdapter(client RagClientInterface) rag_integration_port.RagIntegrationPort {
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
		logger.Logger.ErrorContext(ctx, "RAG UpsertIndex failed", "status", resp.StatusCode(), "body", string(resp.Body))
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

func (a *AugurAdapter) Answer(ctx context.Context, input rag_integration_port.AnswerInput) (<-chan string, error) {
	reqBody := rag_gateway.AnswerRequest{
		Query: input.Query,
	}
	// SessionID is reserved for future use when session context is needed
	_ = input.SessionID
	if len(input.Contexts) > 0 {
		reqBody.CandidateArticleIds = &input.Contexts // Reusing this field if appropriate, or check mapping
	}

	resultChan := make(chan string)

	if input.Stream {
		resp, err := a.client.AnswerWithRAGStream(ctx, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to call AnswerWithRAGStream: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("rag-orchestrator returned status %d", resp.StatusCode)
		}

		go func() {
			defer close(resultChan)
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					// Log but don't fail - data has been streamed
					_ = closeErr
				}
			}()

			// Simple scanner for SSE-like or JSON stream
			// Assuming the backend returns raw chunks or SSE.
			// If it matches the frontend expectation, it might be SSE.
			// Let's assume standard byte stream for now and refine if it's SSE format.
			// Actually the rag-orchestrator client returns the raw response.
			buf := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					resultChan <- string(buf[:n])
				}
				if err != nil {
					break
				}
			}
		}()
	} else {
		resp, err := a.client.AnswerWithRAGWithResponse(ctx, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to call AnswerWithRAG: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("rag-orchestrator returned status %d", resp.StatusCode())
		}
		if resp.JSON200 != nil && resp.JSON200.Answer != nil {
			go func() {
				defer close(resultChan)
				resultChan <- *resp.JSON200.Answer
			}()
		} else {
			close(resultChan)
		}
	}

	return resultChan, nil
}
