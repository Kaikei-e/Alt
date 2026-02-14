// Package backend_api provides a Connect-RPC client for alt-backend's BackendInternalService.
package backend_api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	backendv1 "search-indexer/gen/proto/clients/backend/v1"
	"search-indexer/gen/proto/clients/backend/v1/backendv1connect"

	"search-indexer/driver"
)

// Client wraps the BackendInternalService Connect-RPC client.
// It implements gateway.ArticleDriver to serve as a drop-in replacement
// for the database driver.
type Client struct {
	client       backendv1connect.BackendInternalServiceClient
	serviceToken string
}

const serviceTokenHeader = "X-Service-Token"

// NewClient creates a new backend API client.
func NewClient(baseURL string, serviceToken string) *Client {
	c := backendv1connect.NewBackendInternalServiceClient(
		http.DefaultClient,
		baseURL,
	)
	return &Client{
		client:       c,
		serviceToken: serviceToken,
	}
}

func (c *Client) addAuth(req connect.AnyRequest) {
	if c.serviceToken != "" {
		req.Header().Set(serviceTokenHeader, c.serviceToken)
	}
}

// GetArticlesWithTags fetches articles with backward keyset pagination (backfill).
func (c *Client) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error) {
	protoReq := &backendv1.ListArticlesWithTagsRequest{
		LastId: lastID,
		Limit:  int32(limit),
	}
	if lastCreatedAt != nil {
		protoReq.LastCreatedAt = timestamppb.New(*lastCreatedAt)
	}

	req := connect.NewRequest(protoReq)
	c.addAuth(req)

	resp, err := c.client.ListArticlesWithTags(ctx, req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTags: %w", err)
	}

	articles := toDriverArticles(resp.Msg.Articles)

	var nextCreatedAt *time.Time
	if resp.Msg.NextCreatedAt != nil {
		t := resp.Msg.NextCreatedAt.AsTime()
		nextCreatedAt = &t
	}

	return articles, nextCreatedAt, resp.Msg.NextId, nil
}

// GetArticlesWithTagsForward fetches articles with forward keyset pagination (incremental).
func (c *Client) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error) {
	protoReq := &backendv1.ListArticlesWithTagsForwardRequest{
		LastId: lastID,
		Limit:  int32(limit),
	}
	if incrementalMark != nil {
		protoReq.IncrementalMark = timestamppb.New(*incrementalMark)
	}
	if lastCreatedAt != nil {
		protoReq.LastCreatedAt = timestamppb.New(*lastCreatedAt)
	}

	req := connect.NewRequest(protoReq)
	c.addAuth(req)

	resp, err := c.client.ListArticlesWithTagsForward(ctx, req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTagsForward: %w", err)
	}

	articles := toDriverArticles(resp.Msg.Articles)

	var nextCreatedAt *time.Time
	if resp.Msg.NextCreatedAt != nil {
		t := resp.Msg.NextCreatedAt.AsTime()
		nextCreatedAt = &t
	}

	return articles, nextCreatedAt, resp.Msg.NextId, nil
}

// GetDeletedArticles fetches deleted articles for syncing deletions.
func (c *Client) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*driver.DeletedArticle, *time.Time, error) {
	protoReq := &backendv1.ListDeletedArticlesRequest{
		Limit: int32(limit),
	}
	if lastDeletedAt != nil {
		protoReq.LastDeletedAt = timestamppb.New(*lastDeletedAt)
	}

	req := connect.NewRequest(protoReq)
	c.addAuth(req)

	resp, err := c.client.ListDeletedArticles(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("ListDeletedArticles: %w", err)
	}

	deleted := make([]*driver.DeletedArticle, len(resp.Msg.Articles))
	for i, da := range resp.Msg.Articles {
		deleted[i] = &driver.DeletedArticle{
			ID:        da.Id,
			DeletedAt: da.DeletedAt.AsTime(),
		}
	}

	var nextDeletedAt *time.Time
	if resp.Msg.NextDeletedAt != nil {
		t := resp.Msg.NextDeletedAt.AsTime()
		nextDeletedAt = &t
	}

	return deleted, nextDeletedAt, nil
}

// GetLatestCreatedAt returns the latest article created_at timestamp.
func (c *Client) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	req := connect.NewRequest(&backendv1.GetLatestArticleTimestampRequest{})
	c.addAuth(req)

	resp, err := c.client.GetLatestArticleTimestamp(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetLatestArticleTimestamp: %w", err)
	}

	if resp.Msg.LatestCreatedAt == nil {
		return nil, nil
	}

	t := resp.Msg.LatestCreatedAt.AsTime()
	return &t, nil
}

// GetArticleByID retrieves a single article with tags by ID.
func (c *Client) GetArticleByID(ctx context.Context, articleID string) (*driver.ArticleWithTags, error) {
	req := connect.NewRequest(&backendv1.GetArticleByIDRequest{ArticleId: articleID})
	c.addAuth(req)

	resp, err := c.client.GetArticleByID(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetArticleByID: %w", err)
	}

	return toDriverArticle(resp.Msg.Article), nil
}

func toDriverArticles(protos []*backendv1.ArticleWithTags) []*driver.ArticleWithTags {
	articles := make([]*driver.ArticleWithTags, len(protos))
	for i, p := range protos {
		articles[i] = toDriverArticle(p)
	}
	return articles
}

func toDriverArticle(p *backendv1.ArticleWithTags) *driver.ArticleWithTags {
	tags := make([]driver.TagModel, len(p.Tags))
	for i, t := range p.Tags {
		tags[i] = driver.TagModel{TagName: t}
	}
	return &driver.ArticleWithTags{
		ID:        p.Id,
		Title:     p.Title,
		Content:   p.Content,
		Tags:      tags,
		CreatedAt: p.CreatedAt.AsTime(),
		UserID:    p.UserId,
	}
}
