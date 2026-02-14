package backend_api

import (
	"context"
	"fmt"
	"net/url"

	"connectrpc.com/connect"

	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"

	"pre-processor/domain"
)

// FeedRepository implements repository.FeedRepository using the backend API.
type FeedRepository struct {
	client *Client
}

// NewFeedRepository creates a new API-backed feed repository.
func NewFeedRepository(client *Client) *FeedRepository {
	return &FeedRepository{client: client}
}

// GetUnprocessedFeeds gets unprocessed feeds using cursor-based pagination.
func (r *FeedRepository) GetUnprocessedFeeds(ctx context.Context, cursor *domain.Cursor, limit int) ([]*url.URL, *domain.Cursor, error) {
	var cursorStr string
	if cursor != nil {
		cursorStr = cursor.LastID
	}

	protoReq := &backendv1.ListFeedURLsRequest{
		Cursor: cursorStr,
		Limit:  int32(limit),
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.ListFeedURLs(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("ListFeedURLs: %w", err)
	}

	urls := make([]*url.URL, 0, len(resp.Msg.Feeds))
	for _, f := range resp.Msg.Feeds {
		parsedURL, err := url.Parse(f.Url)
		if err != nil {
			continue
		}
		urls = append(urls, parsedURL)
	}

	var newCursor *domain.Cursor
	if resp.Msg.HasMore && resp.Msg.NextCursor != "" {
		newCursor = &domain.Cursor{
			LastID: resp.Msg.NextCursor,
		}
	}

	return urls, newCursor, nil
}

// GetProcessingStats returns feed processing statistics.
// Not fully available via API - returns basic stats.
func (r *FeedRepository) GetProcessingStats(ctx context.Context) (*domain.ProcessingStatistics, error) {
	// Count total feeds by iterating through all pages
	total := 0
	cursor := ""
	for {
		protoReq := &backendv1.ListFeedURLsRequest{
			Cursor: cursor,
			Limit:  500,
		}
		req := connect.NewRequest(protoReq)
		r.client.addAuth(req)

		resp, err := r.client.client.ListFeedURLs(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("ListFeedURLs: %w", err)
		}

		total += len(resp.Msg.Feeds)
		if !resp.Msg.HasMore {
			break
		}
		cursor = resp.Msg.NextCursor
	}

	return &domain.ProcessingStatistics{
		TotalFeeds:     total,
		ProcessedFeeds: 0,
		RemainingFeeds: total,
	}, nil
}
