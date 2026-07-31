// Package backend_api provides a Connect-RPC client for alt-data-hub's
// DataHubService (ADR-000954 D7). The package name is kept so the DI wiring
// and its tests move in one step; the peer it talks to is alt-data-hub, which
// serves alt.datahub.v1.DataHubService on the same mTLS listener the legacy
// services.backend.v1.BackendInternalService was reached on.
package backend_api

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	datahubv1 "search-indexer/gen/proto/alt/datahub/v1"
	"search-indexer/gen/proto/alt/datahub/v1/datahubv1connect"

	"search-indexer/driver"
)

// Client wraps the DataHubService Connect-RPC client.
// It implements gateway.ArticleDriver to serve as a drop-in replacement
// for the database driver.
type Client struct {
	client datahubv1connect.DataHubServiceClient
}

// DefaultHTTPClient constructs a dedicated *http.Client for backend Connect-RPC
// calls. http.DefaultClient must not be used: its zero Timeout allows a
// hanging alt-backend to exhaust goroutines and file descriptors, and its
// Transport is process-global.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

// NewClient creates a new backend API client. When httpClient is nil a
// reasonable default is used; callers that need mTLS pass a custom client
// built via tlsutil.LoadClientConfig. The serviceToken arg is retained for
// signature compatibility with DI wiring and ignored — authentication is
// established at the TLS transport layer.
func NewClient(baseURL, _ string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = DefaultHTTPClient()
	}
	c := datahubv1connect.NewDataHubServiceClient(
		httpClient,
		baseURL,
	)
	return &Client{client: c}
}

func (c *Client) addAuth(_ connect.AnyRequest) {
	// No-op: authentication is handled by the TLS transport layer (mTLS).
}

// GetArticlesWithTags fetches articles with backward keyset pagination (backfill).
func (c *Client) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error) {
	protoReq := &datahubv1.ListArticlesWithTagsRequest{
		LastId: lastID,
		Limit:  safeInt32(limit),
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
	protoReq := &datahubv1.ListArticlesWithTagsForwardRequest{
		LastId: lastID,
		Limit:  safeInt32(limit),
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
	protoReq := &datahubv1.ListDeletedArticlesRequest{
		Limit: safeInt32(limit),
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
	req := connect.NewRequest(&datahubv1.GetLatestArticleTimestampRequest{})
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
	req := connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: articleID})
	c.addAuth(req)

	resp, err := c.client.GetArticleByID(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetArticleByID: %w", err)
	}

	if resp.Msg.Article == nil {
		return nil, nil
	}

	return toDriverArticle(resp.Msg.Article), nil
}

func toDriverArticles(protos []*datahubv1.ArticleWithTags) []*driver.ArticleWithTags {
	articles := make([]*driver.ArticleWithTags, len(protos))
	for i, p := range protos {
		articles[i] = toDriverArticle(p)
	}
	return articles
}

func toDriverArticle(p *datahubv1.ArticleWithTags) *driver.ArticleWithTags {
	tags := make([]driver.TagModel, len(p.Tags))
	for i, t := range p.Tags {
		tags[i] = driver.TagModel{TagName: t}
	}
	// published_at is the article's source publication timestamp and drives
	// SearchWithDateFilter plus Acolyte's weekly_briefing window. It is unset
	// when the article row has no published_at, in which case created_at is
	// the only date the backend can offer.
	publishedAt := p.CreatedAt.AsTime()
	if p.PublishedAt != nil {
		publishedAt = p.PublishedAt.AsTime()
	}
	return &driver.ArticleWithTags{
		ID:          p.Id,
		Title:       p.Title,
		Content:     p.Content,
		Tags:        tags,
		CreatedAt:   p.CreatedAt.AsTime(),
		UserID:      p.UserId,
		Language:    p.Language,
		PublishedAt: publishedAt,
	}
}

// safeInt32 converts an int to int32 with clamping to prevent overflow.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
