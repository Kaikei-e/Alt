package article_status_port

import (
	"context"
	"net/url"

	"github.com/google/uuid"
)

// UpdateArticleStatusPort marks the feed an article belongs to as read.
//
// userID is a parameter rather than something the implementation digs out of
// the context. The write is served by alt-data-hub since ADR-000954 Wave 3,
// where the peer certificate names alt-backend and not the reader — so the
// tenant has to be carried explicitly, and the usecase is where an
// authenticated request still knows who it belongs to. This also makes the
// port identical in shape to UpdateFeedStatusPort, which is the point of
// capability catalog §4-5: the two writes hit one table and should not differ
// in anything but the lookup key.
type UpdateArticleStatusPort interface {
	MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error
}
