package article_status_port

import (
	"context"
	"net/url"
)

type UpdateArticleStatusPort interface {
	MarkArticleAsRead(ctx context.Context, articleURL url.URL) error
}
