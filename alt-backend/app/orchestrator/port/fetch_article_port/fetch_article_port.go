package fetch_article_port

//go:generate mockgen -source=fetch_article_port.go -destination=../../mocks/mock_fetch_article_port.go -package=mocks

import (
	"context"
)

type FetchArticlePort interface {
	FetchArticleContents(ctx context.Context, articleURL string) (*string, error)
}
