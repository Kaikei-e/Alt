package archive_article_port

//go:generate go run go.uber.org/mock/mockgen -source=archive_port.go -destination=../../mocks/mock_archive_article_port.go -package=mocks ArchiveArticlePort

import "context"

// ArticleRecord represents the minimal data required to persist an article body.
type ArticleRecord struct {
	URL     string
	Title   string
	Content string
}

// ArchiveArticlePort defines the persistence contract for archiving fetched articles.
type ArchiveArticlePort interface {
	SaveArticle(ctx context.Context, record ArticleRecord) error
}
