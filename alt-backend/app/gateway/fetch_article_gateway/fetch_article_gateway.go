package fetch_article_gateway

import (
	"alt/utils/html_parser"
	"alt/utils/rate_limiter"
	"context"
	"io"
	"net/http"
)

type FetchArticleGateway struct {
	rateLimiter *rate_limiter.HostRateLimiter
	httpClient  *http.Client
}

func NewFetchArticleGateway(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client) *FetchArticleGateway {
	return &FetchArticleGateway{
		rateLimiter: rateLimiter,
		httpClient:  httpClient,
	}
}

func (g *FetchArticleGateway) FetchArticleContents(ctx context.Context, articleURL string) (*string, error) {
	resp, err := g.httpClient.Get(articleURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	content := string(body)
	content = html_parser.StripTags(content)

	return &content, nil
}
