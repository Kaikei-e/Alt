// Package article_url_lookup_port defines a tenant-scoped lookup of an
// article's canonical source URL by its article ID. Used at event-append time
// to enrich knowledge_event payloads with the URL the projector needs (so the
// projector itself stays reproject-safe and never touches the latest article
// state).
package article_url_lookup_port

import (
	"context"

	"github.com/google/uuid"
)

// ArticleURLLookupPort returns the canonical source URL for an article scoped
// to the calling user. Returns ("", nil) when the article does not exist or
// belongs to another user; the caller decides whether to log + fall back.
//
// Tenant isolation: the implementation MUST filter on user_id (not just
// article_id) so cross-tenant URL disclosure is impossible (security audit
// finding #1 on the ACT Open canonical fix).
type ArticleURLLookupPort interface {
	LookupArticleURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error)
}
