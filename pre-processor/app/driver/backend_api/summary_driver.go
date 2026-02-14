package backend_api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"

	"pre-processor/domain"
)

// SummaryRepository implements repository.SummaryRepository using the backend API.
type SummaryRepository struct {
	client *Client
}

// NewSummaryRepository creates a new API-backed summary repository.
func NewSummaryRepository(client *Client) *SummaryRepository {
	return &SummaryRepository{client: client}
}

// Create creates a new article summary via the backend API.
func (r *SummaryRepository) Create(ctx context.Context, summary *domain.ArticleSummary) error {
	if summary == nil {
		return fmt.Errorf("summary cannot be nil")
	}
	if summary.ArticleID == "" {
		return fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &backendv1.SaveArticleSummaryRequest{
		ArticleId: summary.ArticleID,
		Summary:   summary.SummaryJapanese,
		Language:  "ja",
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	_, err := r.client.client.SaveArticleSummary(ctx, req)
	if err != nil {
		return fmt.Errorf("SaveArticleSummary: %w", err)
	}

	return nil
}

// FindArticlesWithSummaries finds articles with summaries for quality checking.
// This operation requires complex DB joins not available via API.
func (r *SummaryRepository) FindArticlesWithSummaries(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	// Quality checking uses direct DB access; not needed in API mode
	return nil, nil, nil
}

// Delete deletes an article summary.
// Not available via API in the current phase.
func (r *SummaryRepository) Delete(ctx context.Context, summaryID string) error {
	return fmt.Errorf("Delete not available via backend API")
}

// Exists checks if an article summary exists.
// Not available via API in the current phase.
func (r *SummaryRepository) Exists(ctx context.Context, summaryID string) (bool, error) {
	return false, fmt.Errorf("Exists not available via backend API")
}
