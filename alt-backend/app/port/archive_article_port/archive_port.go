package archive_article_port

//go:generate go run go.uber.org/mock/mockgen -source=archive_port.go -destination=../../mocks/mock_archive_article_port.go -package=mocks ArchiveArticlePort

import "context"

// ArticleRecord represents the minimal data required to persist an article body.
type ArticleRecord struct {
	URL     string
	Title   string
	Content string
}

// ArticleSaver defines the interface for saving articles
type ArticleSaver interface {
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

// ArchiveArticlePort defines the interface for archiving articles using ArticleRecord
type ArchiveArticlePort interface {
	SaveArticle(ctx context.Context, record ArticleRecord) error
}
