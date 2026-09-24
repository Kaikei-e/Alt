package repository

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"pre-processor/domain"
	backend_api "pre-processor/driver/backend_api"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
)

// summaryRepository implements SummaryRepository using the backend API.
type summaryRepository struct {
	client *backend_api.Client
}

// NewSummaryRepository creates a new API-backed summary repository.
func NewSummaryRepository(client *backend_api.Client) *summaryRepository {
	return &summaryRepository{client: client}
}

// Create creates a new article summary via the backend API.
func (r *summaryRepository) Create(ctx context.Context, summary *domain.ArticleSummary) error {
	if summary == nil {
		return fmt.Errorf("summary cannot be nil")
	}
	if summary.ArticleID == "" {
		return fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &datahubv1.SaveArticleSummaryRequest{
		ArticleId: summary.ArticleID,
		Summary:   summary.SummaryJapanese,
		Language:  "ja",
		UserId:    summary.UserID,
	}

	req := connect.NewRequest(protoReq)
	r.client.AddAuth(req)

	_, err := r.client.DataHub().SaveArticleSummary(ctx, req)
	if err != nil {
		return fmt.Errorf("SaveArticleSummary: %w", err)
	}

	return nil
}

// FindArticlesWithSummaries finds articles with summaries for quality checking via the backend API.
func (r *summaryRepository) FindArticlesWithSummaries(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	protoReq := &datahubv1.FindArticlesWithSummariesRequest{
		Limit: int32(min(limit, math.MaxInt32)), // #nosec G115 -- clamped to int32 range
	}

	if cursor != nil {
		if cursor.LastCreatedAt != nil {
			protoReq.LastCreatedAt = timestamppb.New(*cursor.LastCreatedAt)
		}
		protoReq.LastId = cursor.LastID
	}

	req := connect.NewRequest(protoReq)
	r.client.AddAuth(req)

	resp, err := r.client.DataHub().FindArticlesWithSummaries(ctx, req)
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
func (r *summaryRepository) Delete(ctx context.Context, articleID string) error {
	if articleID == "" {
		return fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &datahubv1.DeleteArticleSummaryRequest{
		ArticleId: articleID,
	}

	req := connect.NewRequest(protoReq)
	r.client.AddAuth(req)

	_, err := r.client.DataHub().DeleteArticleSummary(ctx, req)
	if err != nil {
		return fmt.Errorf("DeleteArticleSummary: %w", err)
	}

	return nil
}

// Exists checks if an article summary exists via the backend API.
func (r *summaryRepository) Exists(ctx context.Context, articleID string) (bool, error) {
	if articleID == "" {
		return false, fmt.Errorf("article ID cannot be empty")
	}

	protoReq := &datahubv1.CheckArticleSummaryExistsRequest{
		ArticleId: articleID,
	}

	req := connect.NewRequest(protoReq)
	r.client.AddAuth(req)

	resp, err := r.client.DataHub().CheckArticleSummaryExists(ctx, req)
	if err != nil {
		return false, fmt.Errorf("CheckArticleSummaryExists: %w", err)
	}

	return resp.Msg.Exists, nil
}
