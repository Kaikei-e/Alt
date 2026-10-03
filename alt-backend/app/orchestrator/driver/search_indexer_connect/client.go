// Package search_indexer_connect provides Connect-RPC client for search-indexer service.
package search_indexer_connect

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	searchv2 "alt/gen/proto/services/search/v2"
	"alt/gen/proto/services/search/v2/searchv2connect"
	"alt/shared/domain/authcontext"
	"alt/tlsutil"
	"alt/utils/safeconv"
)

type authInterceptor struct{}

func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if jwtToken, ok := authcontext.JWTFromContext(ctx); ok && jwtToken != "" {
			req.Header().Set("X-Alt-Backend-Token", jwtToken)
		}
		return next(ctx, req)
	}
}

func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if jwtToken, ok := authcontext.JWTFromContext(ctx); ok && jwtToken != "" {
			conn.RequestHeader().Set("X-Alt-Backend-Token", jwtToken)
		}
		return conn
	}
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func defaultSearchHTTPClient(baseURL string) (connect.HTTPClient, error) {
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("search-indexer baseURL must use https:// scheme, got: %s", baseURL)
	}
	certFile := os.Getenv("MTLS_CERT_FILE")
	keyFile := os.Getenv("MTLS_KEY_FILE")
	caFile := os.Getenv("MTLS_CA_FILE")

	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("missing MTLS cert/key/ca environment variables")
	}

	tlsCfg, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS client config: %w", err)
	}

	serverName := os.Getenv("SEARCH_INDEXER_MTLS_SERVER_NAME")
	if serverName == "" {
		serverName = "search-indexer"
	}
	tlsCfg.ServerName = serverName
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// ArticleHit represents a raw search hit from search-indexer.
type ArticleHit struct {
	ID      string
	Title   string
	Content string
	Tags    []string
}

// RecapHit represents a raw recap search result from search-indexer.
type RecapHit struct {
	JobID      string
	ExecutedAt string
	WindowDays int
	Genre      string
	Summary    string
	TopTerms   []string
	Tags       []string
	Bullets    []string
}

// Client provides Connect-RPC client for search-indexer.
type Client struct {
	client searchv2connect.SearchServiceClient
}

// NewClient creates a new Connect-RPC client for search-indexer.
func NewClient(baseURL string) (*Client, error) {
	httpClient, err := defaultSearchHTTPClient(baseURL)
	if err != nil {
		return nil, err
	}
	client := searchv2connect.NewSearchServiceClient(
		httpClient,
		baseURL,
		connect.WithProtoJSON(),
		connect.WithInterceptors(&authInterceptor{}),
	)
	return &Client{client: client}, nil
}

// NewClientWithHTTPClient creates a client with an injected HTTP client, for tests only.
func NewClientWithHTTPClient(baseURL string, httpClient connect.HTTPClient) *Client {
	client := searchv2connect.NewSearchServiceClient(
		httpClient,
		baseURL,
		connect.WithProtoJSON(),
		connect.WithInterceptors(&authInterceptor{}),
	)
	return &Client{client: client}
}

// SearchArticles searches for articles matching the query via Connect-RPC.
func (d *Client) SearchArticles(ctx context.Context, query string, userID string) ([]ArticleHit, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Limit:  20,
	}))
	if err != nil {
		return nil, err
	}

	hits := make([]ArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = ArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, nil
}

// SearchArticlesWithPagination searches for articles with pagination support via Connect-RPC.
func (d *Client) SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]ArticleHit, int64, error) {
	resp, err := d.client.SearchArticles(ctx, connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  query,
		UserId: userID,
		Offset: safeconv.Int32(offset),
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	hits := make([]ArticleHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		hits[i] = ArticleHit{
			ID:      hit.Id,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return hits, resp.Msg.EstimatedTotalHits, nil
}

// SearchRecapsByTag searches recap genres by tag name via search-indexer's Meilisearch.
func (d *Client) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]RecapHit, error) {
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		TagName: tagName,
		Limit:   safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, err
	}

	results := make([]RecapHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = RecapHit{
			JobID:      hit.JobId,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: int(hit.WindowDays),
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}

	return results, nil
}

// SearchRecapsByQuery searches recap genres by free-text query via search-indexer's Meilisearch.
func (d *Client) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]RecapHit, int64, error) {
	q := &query
	resp, err := d.client.SearchRecaps(ctx, connect.NewRequest(&searchv2.SearchRecapsRequest{
		Query: q,
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, 0, err
	}

	results := make([]RecapHit, len(resp.Msg.Hits))
	for i, hit := range resp.Msg.Hits {
		results[i] = RecapHit{
			JobID:      hit.JobId,
			ExecutedAt: hit.ExecutedAt,
			WindowDays: int(hit.WindowDays),
			Genre:      hit.Genre,
			Summary:    hit.Summary,
			TopTerms:   hit.TopTerms,
			Tags:       hit.Tags,
			Bullets:    hit.Bullets,
		}
	}

	return results, resp.Msg.EstimatedTotalHits, nil
}
