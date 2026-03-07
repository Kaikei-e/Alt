package validate_fetch_rss_port

import (
	"alt/domain"
	"context"
)

// ValidateAndFetchRSSPort validates an RSS URL (SSRF checks, format validation)
// and fetches the feed in a single operation. This is the external HTTP boundary
// for feed registration — the only place where an outbound RSS fetch occurs.
type ValidateAndFetchRSSPort interface {
	ValidateAndFetch(ctx context.Context, link string) (*domain.ParsedFeed, error)
}
