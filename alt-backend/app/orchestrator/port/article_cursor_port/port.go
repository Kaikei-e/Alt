package article_cursor_port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type FetchArticleCursorPort interface {
	FetchArticleIDsWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]uuid.UUID, error)
}
