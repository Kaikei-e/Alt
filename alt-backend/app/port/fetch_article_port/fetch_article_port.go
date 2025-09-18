package fetch_article_port

//go:generate mockgen -source=fetch_article_port.go -destination=../../mocks/mock_fetch_article_port.go -package=mocks

import (
	"alt/utils/rate_limiter"
	"context"
	"net/http"
)

type FetchArticlePort interface {
	FetchArticleContents(ctx context.Context, articleURL string) (*string, error)
}

type FetchArticlePortImpl struct {
	rateLimiter *rate_limiter.HostRateLimiter
	httpClient  *http.Client
}

func NewFetchArticlePortImpl(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client) *FetchArticlePortImpl {
	return &FetchArticlePortImpl{
		rateLimiter: rateLimiter,
		httpClient:  httpClient,
	}
}
