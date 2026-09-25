package article_content_extractor_port

import "context"

// ArticleContentExtractorPort fetches and extracts article content and title from a web URL.
type ArticleContentExtractorPort interface {
	ExtractArticleContent(ctx context.Context, urlStr string) (content string, title string, err error)
}
