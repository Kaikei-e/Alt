package backend_api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		UserId:    summary.UserID,
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	_, err := r.client.client.SaveArticleSummary(ctx, req)
	if err != nil {
		return fmt.Errorf("SaveArticleSummary: %w", err)
	}

	return nil
}

// FindArticlesWithSummaries finds articles with summaries for quality checking via the backend API.
func (r *SummaryRepository) FindArticlesWithSummaries(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	protoReq := &backendv1.FindArticlesWithSummariesRequest{
		Limit: int32(limit),
	}

	if cursor != nil {
		if cursor.LastCreatedAt != nil {
			protoReq.LastCreatedAt = timestamppb.New(*cursor.LastCreatedAt)
		}
		protoReq.LastId = cursor.LastID
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.FindArticlesWithSummaries(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("FindArticlesWithSummaries: %w", err)
	}

	results := make([]*domain.ArticleWithSummary, len(resp.Msg.Articles))
	for i, a := range resp.Msg.Articles {
		results[i] = &domain.ArticleWithSummary{
			ArticleID:       a.ArticleId,
			ArticleContent:  a.ArticleContent,
			ArticleURL:      a.ArticleUrl,
			SummaryID:       a.SummaryId,
			SummaryJapanese: a.SummaryJapanese,
		}
		if a.CreatedAt != nil {
			results[i].CreatedAt = a.CreatedAt.AsTime()
		}
	}

	var newCursor *domain.Cursor
	if resp.Msg.NextId != "" {
		newCursor = &domain.Cursor{
			LastID: resp.Msg.NextId,
		}
		if resp.Msg.NextCreatedAt != nil {
			t := resp.Msg.NextCreatedAt.AsTime()
			newCursor.LastCreatedAt = &t
		}
	}

	return results, newCursor, nil
}

// Delete deletes an article summary by article ID via the backend API.
func (r *SummaryRepository) Delete(ctx context.Context, articleID string) error {
	if articleID == "" {
		return fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &backendv1.DeleteArticleSummaryRequest{
		ArticleId: articleID,
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	_, err := r.client.client.DeleteArticleSummary(ctx, req)
	if err != nil {
		return fmt.Errorf("DeleteArticleSummary: %w", err)
	}

	return nil
}

// Exists checks if an article summary exists via the backend API.
func (r *SummaryRepository) Exists(ctx context.Context, articleID string) (bool, error) {
	if articleID == "" {
		return false, fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &backendv1.CheckArticleSummaryExistsRequest{
		ArticleId: articleID,
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.CheckArticleSummaryExists(ctx, req)
	if err != nil {
		return false, fmt.Errorf("CheckArticleSummaryExists: %w", err)
	}

	return resp.Msg.Exists, nil
}
